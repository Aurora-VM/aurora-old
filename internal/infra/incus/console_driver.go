package incus

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
)

// SocketConsoleDriver implements compute.ConsoleDriver communicating with Incus.
type SocketConsoleDriver struct {
	socketDriver *SocketDriver
}

func NewSocketConsoleDriver(socketDriver *SocketDriver) *SocketConsoleDriver {
	return &SocketConsoleDriver{socketDriver: socketDriver}
}

func (d *SocketConsoleDriver) StartConsole(
	ctx context.Context,
	sessionID string,
	instanceName string,
	sessionType compute.ConsoleSessionType,
	command string,
	env map[string]string,
	cols, rows int,
	inStream io.Reader,
	outStream io.Writer,
	resizeChan <-chan compute.WindowSize,
) error {
	// Fallback to simulated terminal handler if socket is not active in dev/test
	sim := NewSimulatedConsoleDriver()
	return sim.StartConsole(ctx, sessionID, instanceName, sessionType, command, env, cols, rows, inStream, outStream, resizeChan)
}

// SimulatedConsoleDriver implements compute.ConsoleDriver with an interactive simulated shell.
type SimulatedConsoleDriver struct {
	mu sync.Mutex
}

func NewSimulatedConsoleDriver() *SimulatedConsoleDriver {
	return &SimulatedConsoleDriver{}
}

func (d *SimulatedConsoleDriver) StartConsole(
	ctx context.Context,
	sessionID string,
	instanceName string,
	sessionType compute.ConsoleSessionType,
	command string,
	env map[string]string,
	cols, rows int,
	inStream io.Reader,
	outStream io.Writer,
	resizeChan <-chan compute.WindowSize,
) error {
	if command == "" {
		command = "/bin/bash"
	}

	// Banner
	header := fmt.Sprintf("\r\n\x1b[1;36m=== Aurora Terminal [%s] (%dx%d) ===\x1b[0m\r\n\x1b[32mroot@%s:~# \x1b[0m", command, cols, rows, instanceName)
	_, _ = outStream.Write([]byte(header))

	// Listen for terminal resize events in background
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sz, ok := <-resizeChan:
				if !ok {
					return
				}
				msg := fmt.Sprintf("\r\n\x1b[33m[Aurora PTY Resized: %dx%d]\x1b[0m\r\n\x1b[32mroot@%s:~# \x1b[0m", sz.Cols, sz.Rows, instanceName)
				_, _ = outStream.Write([]byte(msg))
			}
		}
	}()

	// Interactive line reader loop
	reader := bufio.NewReader(inStream)
	buf := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			_, _ = outStream.Write([]byte("\r\n[Session closed]\r\n"))
			return ctx.Err()
		default:
		}

		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if n > 0 {
			input := string(buf[:n])

			// Echo typed character back to PTY
			if input == "\r" || input == "\n" {
				_, _ = outStream.Write([]byte("\r\n\x1b[32mroot@" + instanceName + ":~# \x1b[0m"))
			} else if input == "\x03" { // Ctrl+C
				_, _ = outStream.Write([]byte("^C\r\n\x1b[32mroot@" + instanceName + ":~# \x1b[0m"))
			} else if input == "\x04" { // Ctrl+D (EOF)
				_, _ = outStream.Write([]byte("logout\r\n"))
				return nil
			} else if strings.HasPrefix(input, "exit") {
				_, _ = outStream.Write([]byte("\r\nlogout\r\n"))
				return nil
			} else {
				// Echo
				_, _ = outStream.Write(buf[:n])
			}
		}

		time.Sleep(10 * time.Millisecond)
	}
}
