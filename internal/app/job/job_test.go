package job_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	appJob "github.com/aurora-vm/aurora/internal/app/job"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

type mockAuthorizer struct{}

func (m *mockAuthorizer) Authorize(ctx context.Context, sub *domainIdentity.Subject, action string, res *domainIdentity.Resource) error {
	return nil
}

func TestJobEngine_SubmitAndExecuteSuccess(t *testing.T) {
	memStore := memory.NewMemoryStore()
	jobRepo := memStore.Jobs()
	leaseRepo := memStore.Leases()
	authorizer := &mockAuthorizer{}

	engine := appJob.NewEngine(jobRepo, leaseRepo, authorizer, nil, 2)
	defer engine.Close()

	var executed atomic.Bool
	engine.RegisterHandler("instance.provision", func(ctx context.Context, j *domainJob.Job, r appJob.ProgressReporter) (json.RawMessage, error) {
		_ = r.UpdateProgress(ctx, 50)
		time.Sleep(50 * time.Millisecond)
		_ = r.UpdateProgress(ctx, 100)
		executed.Store(true)
		return json.Marshal(map[string]string{"instanceId": "inst-123"})
	})

	sub := &domainIdentity.Subject{UserID: "user-1", Roles: []string{"admin"}}
	j, err := engine.Submit(context.Background(), sub, &domainJob.Job{
		Type:         "instance.provision",
		ResourceType: "instance",
		ResourceID:   "inst-123",
	})
	if err != nil {
		t.Fatalf("unexpected error submitting job: %v", err)
	}

	// Wait for job completion
	var finalJob *domainJob.Job
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		finalJob, _ = engine.GetJob(context.Background(), sub, j.ID)
		if finalJob.Status == domainJob.StatusSucceeded {
			break
		}
	}

	if finalJob.Status != domainJob.StatusSucceeded {
		t.Fatalf("expected job to succeed, got status: %s (error: %s)", finalJob.Status, finalJob.Error)
	}
	if !executed.Load() {
		t.Fatalf("expected handler to execute")
	}
	if finalJob.ProgressPercent != 100 {
		t.Fatalf("expected progress 100, got %d", finalJob.ProgressPercent)
	}
}

func TestJobEngine_RetryOnFailure(t *testing.T) {
	memStore := memory.NewMemoryStore()
	jobRepo := memStore.Jobs()
	leaseRepo := memStore.Leases()
	authorizer := &mockAuthorizer{}

	engine := appJob.NewEngine(jobRepo, leaseRepo, authorizer, nil, 2)
	defer engine.Close()

	var attempts atomic.Int32
	engine.RegisterHandler("node.drain", func(ctx context.Context, j *domainJob.Job, r appJob.ProgressReporter) (json.RawMessage, error) {
		count := attempts.Add(1)
		if count == 1 {
			return nil, errors.New("transient node error")
		}
		return json.Marshal(map[string]string{"status": "drained"})
	})

	sub := &domainIdentity.Subject{UserID: "user-1", Roles: []string{"admin"}}
	j, err := engine.Submit(context.Background(), sub, &domainJob.Job{
		Type:       "node.drain",
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Wait for first failure & retrying state
	var failedJob *domainJob.Job
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		failedJob, _ = engine.GetJob(context.Background(), sub, j.ID)
		if failedJob.Status == domainJob.StatusRetrying || failedJob.RetryCount > 0 {
			break
		}
	}

	if failedJob.RetryCount == 0 {
		t.Fatalf("expected retry count > 0, got %d", failedJob.RetryCount)
	}
}

func TestJobEngine_CancelAndRetryOperations(t *testing.T) {
	memStore := memory.NewMemoryStore()
	jobRepo := memStore.Jobs()
	leaseRepo := memStore.Leases()
	authorizer := &mockAuthorizer{}

	engine := appJob.NewEngine(jobRepo, leaseRepo, authorizer, nil, 1)
	defer engine.Close()

	sub := &domainIdentity.Subject{UserID: "user-1", Roles: []string{"admin"}}
	j, err := engine.Submit(context.Background(), sub, &domainJob.Job{
		Type: "unhandled.type",
	})
	if err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Cancel the job
	err = engine.CancelJob(context.Background(), sub, j.ID, "user requested stop")
	if err != nil {
		t.Fatalf("failed to cancel job: %v", err)
	}

	canceled, _ := engine.GetJob(context.Background(), sub, j.ID)
	if canceled.Status != domainJob.StatusCanceled {
		t.Fatalf("expected status canceled, got: %s", canceled.Status)
	}

	// Retry the canceled job
	retried, err := engine.RetryJob(context.Background(), sub, j.ID)
	if err != nil {
		t.Fatalf("failed to retry canceled job: %v", err)
	}
	if retried.Status != domainJob.StatusPending {
		t.Fatalf("expected status pending on retry, got: %s", retried.Status)
	}
}
