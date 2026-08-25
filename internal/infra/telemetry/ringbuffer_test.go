package telemetry

import (
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/monitoring"
	"github.com/stretchr/testify/assert"
)

func TestRingBuffer_Push_And_QueryRange(t *testing.T) {
	rb := NewRingBuffer(100)

	baseTime := time.Now().UTC().Truncate(time.Minute)

	samples := []*monitoring.MetricSample{
		{ResourceType: monitoring.ResourceTypeInstance, ResourceID: "inst-1", MetricName: "cpu_percent", Value: 20.0, Timestamp: baseTime.Add(10 * time.Second)},
		{ResourceType: monitoring.ResourceTypeInstance, ResourceID: "inst-1", MetricName: "cpu_percent", Value: 30.0, Timestamp: baseTime.Add(15 * time.Second)},
		{ResourceType: monitoring.ResourceTypeInstance, ResourceID: "inst-1", MetricName: "cpu_percent", Value: 50.0, Timestamp: baseTime.Add(30 * time.Second)},
		{ResourceType: monitoring.ResourceTypeInstance, ResourceID: "inst-2", MetricName: "cpu_percent", Value: 80.0, Timestamp: baseTime.Add(10 * time.Second)},
	}

	rb.PushBatch(samples)

	ts := rb.QueryRange(monitoring.ResourceTypeInstance, "inst-1", "cpu_percent", baseTime, baseTime.Add(1*time.Minute), 10)
	assert.Equal(t, "cpu_percent", ts.MetricName)
	assert.Equal(t, "%", ts.Unit)
	assert.NotEmpty(t, ts.DataPoints)

	// Bucket 10s has 20.0 and 30.0 -> avg = 25.0
	assert.Equal(t, 25.0, ts.DataPoints[0].Value)
}
