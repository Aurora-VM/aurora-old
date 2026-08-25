package monitoring

import "time"

type ResourceType string

const (
	ResourceTypeNode     ResourceType = "node"
	ResourceTypeInstance ResourceType = "instance"
)

// MetricSample represents a single telemetry metric measurement at a point in time.
type MetricSample struct {
	ResourceType ResourceType `json:"resourceType"`
	ResourceID   string       `json:"resourceId"`
	MetricName   string       `json:"metricName"`
	Value        float64      `json:"value"`
	Timestamp    time.Time    `json:"timestamp"`
}

// DataPoint represents a timestamp-value pair in a time-series curve.
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// TimeSeries represents an aggregated sequence of metrics over a time window.
type TimeSeries struct {
	MetricName string      `json:"metricName"`
	Unit       string      `json:"unit"`
	DataPoints []DataPoint `json:"dataPoints"`
}
