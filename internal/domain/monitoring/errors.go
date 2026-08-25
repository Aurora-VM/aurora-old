package monitoring

import "errors"

var (
	ErrThresholdNotFound     = errors.New("alert threshold not found")
	ErrAlertEventNotFound    = errors.New("alert event not found")
	ErrInvalidThresholdSpec  = errors.New("invalid alert threshold specification")
	ErrUnsupportedMetricName = errors.New("unsupported metric name")
	ErrInvalidOperator       = errors.New("invalid alert comparison operator")
)
