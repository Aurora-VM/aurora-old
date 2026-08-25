package compute

import (
	"context"
	"io"
	"time"
)

type ConsoleSessionType string

const (
	ConsoleTypeExec ConsoleSessionType = "exec"
	ConsoleTypeVNC  ConsoleSessionType = "vnc"
)

type WindowSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// ConsoleSession tracks active interactive terminal or VNC sessions for an instance.
type ConsoleSession struct {
	ID         string             `json:"id"`
	InstanceID string             `json:"instanceId"`
	UserID     string             `json:"userId"`
	Type       ConsoleSessionType `json:"type"`
	Command    string             `json:"command"`
	Cols       int                `json:"cols"`
	Rows       int                `json:"rows"`
	Active     bool               `json:"active"`
	CreatedAt  time.Time          `json:"createdAt"`
	ClosedAt   *time.Time         `json:"closedAt,omitempty"`
}

// ConsoleDriver defines hypervisor driver capabilities for interactive PTY and VNC consoles.
type ConsoleDriver interface {
	StartConsole(
		ctx context.Context,
		sessionID string,
		instanceName string,
		sessionType ConsoleSessionType,
		command string,
		env map[string]string,
		cols, rows int,
		inStream io.Reader,
		outStream io.Writer,
		resizeChan <-chan WindowSize,
	) error
}
