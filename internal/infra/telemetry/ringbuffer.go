package telemetry

import (
	"math"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/monitoring"
)

// RingBuffer implements a thread-safe circular ring buffer for fast telemetry metrics.
type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	samples  []*monitoring.MetricSample
	head     int
	size     int
}

// NewRingBuffer creates a RingBuffer with a fixed capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 50000 // default to 50k samples in RAM
	}
	return &RingBuffer{
		capacity: capacity,
		samples:  make([]*monitoring.MetricSample, capacity),
	}
}

// Push adds a new metric sample to the circular buffer.
func (rb *RingBuffer) Push(sample *monitoring.MetricSample) {
	if sample == nil {
		return
	}
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.samples[rb.head] = sample
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

// PushBatch adds multiple samples to the circular buffer.
func (rb *RingBuffer) PushBatch(samples []*monitoring.MetricSample) {
	if len(samples) == 0 {
		return
	}
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, s := range samples {
		if s == nil {
			continue
		}
		rb.samples[rb.head] = s
		rb.head = (rb.head + 1) % rb.capacity
		if rb.size < rb.capacity {
			rb.size++
		}
	}
}

// QueryRange filters and downsamples time-series metrics from the ring buffer.
func (rb *RingBuffer) QueryRange(
	resType monitoring.ResourceType,
	resID, metricName string,
	from, to time.Time,
	stepSeconds int,
) *monitoring.TimeSeries {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if stepSeconds <= 0 {
		stepSeconds = 10 // default 10-second buckets
	}

	// Filter matching samples in the time range
	var matching []*monitoring.MetricSample
	for i := 0; i < rb.size; i++ {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		s := rb.samples[idx]
		if s == nil {
			continue
		}
		if s.ResourceType == resType && s.ResourceID == resID && s.MetricName == metricName {
			if !s.Timestamp.Before(from) && !s.Timestamp.After(to) {
				matching = append(matching, s)
			}
		}
	}

	if len(matching) == 0 {
		return &monitoring.TimeSeries{
			MetricName: metricName,
			Unit:       getMetricUnit(metricName),
			DataPoints: []monitoring.DataPoint{},
		}
	}

	// Group into step buckets
	type bucket struct {
		sum   float64
		count int
	}
	buckets := make(map[int64]*bucket)

	for _, s := range matching {
		bucketKey := (s.Timestamp.Unix() / int64(stepSeconds)) * int64(stepSeconds)
		if b, ok := buckets[bucketKey]; ok {
			b.sum += s.Value
			b.count++
		} else {
			buckets[bucketKey] = &bucket{sum: s.Value, count: 1}
		}
	}

	// Build sorted chronologically ascending data points
	var points []monitoring.DataPoint
	startBucket := (from.Unix() / int64(stepSeconds)) * int64(stepSeconds)
	endBucket := (to.Unix() / int64(stepSeconds)) * int64(stepSeconds)

	for t := startBucket; t <= endBucket; t += int64(stepSeconds) {
		if b, ok := buckets[t]; ok && b.count > 0 {
			avg := b.sum / float64(b.count)
			points = append(points, monitoring.DataPoint{
				Timestamp: time.Unix(t, 0).UTC(),
				Value:     math.Round(avg*100) / 100,
			})
		}
	}

	return &monitoring.TimeSeries{
		MetricName: metricName,
		Unit:       getMetricUnit(metricName),
		DataPoints: points,
	}
}

func getMetricUnit(metricName string) string {
	switch metricName {
	case "cpu_percent":
		return "%"
	case "memory_used_bytes", "memory_total_bytes", "disk_used_bytes", "disk_total_bytes", "net_rx_bytes", "net_tx_bytes":
		return "bytes"
	case "load_1m", "load_5m", "load_15m":
		return "load"
	default:
		return "value"
	}
}
