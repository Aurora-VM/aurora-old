package siem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
)

// Dispatcher coordinates real-time export of audit logs to SIEM systems.
type Dispatcher struct {
	siemRepo   audit.SIEMRepository
	httpClient *http.Client
	logChan    chan *audit.AuditLog
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewDispatcher creates an asynchronous SIEM dispatcher with worker queues.
func NewDispatcher(siemRepo audit.SIEMRepository, bufferSize int) *Dispatcher {
	if bufferSize <= 0 {
		bufferSize = 2000
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		siemRepo: siemRepo,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logChan: make(chan *audit.AuditLog, bufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}

	d.wg.Add(1)
	go d.worker()

	return d
}

// Dispatch queues an audit log for asynchronous delivery.
func (d *Dispatcher) Dispatch(log *audit.AuditLog) {
	if log == nil {
		return
	}
	select {
	case d.logChan <- log:
	default:
		// Queue full -> non-blocking drop to prevent blocking the main request path
	}
}

// Close gracefully stops the dispatcher and drains the worker.
func (d *Dispatcher) Close() {
	d.cancel()
	close(d.logChan)
	d.wg.Wait()
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()

	for log := range d.logChan {
		if log == nil {
			continue
		}
		destinations, err := d.siemRepo.List(d.ctx)
		if err != nil {
			continue
		}

		for _, dest := range destinations {
			if !dest.Enabled {
				continue
			}
			_ = d.forward(d.ctx, dest, log)
		}
	}
}

func (d *Dispatcher) forward(ctx context.Context, dest *audit.SIEMDestination, log *audit.AuditLog) error {
	payload, err := FormatLog(dest.Format, log)
	if err != nil {
		return err
	}

	switch dest.Type {
	case audit.SIEMTypeWebhook:
		req, err := http.NewRequestWithContext(ctx, "POST", dest.Target, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if dest.AuthToken != "" {
			req.Header.Set("Authorization", "Bearer "+dest.AuthToken)
		}
		resp, err := d.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil

	case audit.SIEMTypeSyslogTCP:
		conn, err := net.DialTimeout("tcp", dest.Target, 3*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write(append(payload, '\n'))
		return err

	case audit.SIEMTypeSyslogUDP:
		conn, err := net.DialTimeout("udp", dest.Target, 3*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write(append(payload, '\n'))
		return err

	default:
		return fmt.Errorf("unsupported SIEM transport type: %s", dest.Type)
	}
}

// FormatLog serializes an audit log into the requested SIEM output format.
func FormatLog(format audit.SIEMFormat, a *audit.AuditLog) ([]byte, error) {
	switch format {
	case audit.SIEMFormatCEF:
		// CEF:0|Aurora|ControlPlane|0.1.0|action|action|severity|extensions
		actor := "system"
		if a.ActorID != nil {
			actor = *a.ActorID
		}
		resID := ""
		if a.ResourceID != nil {
			resID = *a.ResourceID
		}
		cef := fmt.Sprintf(
			"CEF:0|Aurora|ControlPlane|0.1.0|%s|%s|%s|src=%s suser=%s cs1=%s cs1Label=ResourceType cs2=%s cs2Label=ResourceID",
			a.Action, a.Action, string(a.Severity), a.ActorIP, actor, a.ResourceType, resID,
		)
		return []byte(cef), nil

	case audit.SIEMFormatRFC5424:
		// <14>1 2026-08-24T12:00:00Z aurora controlplane 1234 ID47 [aurora@4242 action="..."] msg
		actor := "system"
		if a.ActorID != nil {
			actor = *a.ActorID
		}
		detailsJSON, _ := json.Marshal(a.Details)
		syslog := fmt.Sprintf(
			"<134>1 %s aurora controlplane %d %s [aurora@4242 action=\"%s\" actor=\"%s\" ip=\"%s\"] %s",
			a.CreatedAt.Format(time.RFC3339Nano),
			a.ID,
			a.Action,
			a.Action,
			actor,
			a.ActorIP,
			string(detailsJSON),
		)
		return []byte(syslog), nil

	case audit.SIEMFormatJSON:
		fallthrough
	default:
		return json.Marshal(a)
	}
}
