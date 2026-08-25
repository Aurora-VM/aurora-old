package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	wsURL := flag.String("url", "", "WebSocket target URL")
	cmd := flag.String("cmd", "echo 'Hello from Aurora Web Terminal!'\n", "Command to send")
	flag.Parse()

	if *wsURL == "" {
		log.Fatal("WebSocket URL is required (-url)")
	}

	dialer := websocket.DefaultDialer
	header := make(http.Header)
	conn, resp, err := dialer.Dial(*wsURL, header)
	if err != nil {
		if resp != nil {
			log.Fatalf("WebSocket connection failed (status %d): %v", resp.StatusCode, err)
		}
		log.Fatalf("WebSocket connection failed: %v", err)
	}
	defer conn.Close()

	fmt.Println("\x1b[32m[Connected to Aurora WebSocket Interactive Console]\x1b[0m")

	// Read Banner
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, msg, err := conn.ReadMessage()
	if err == nil {
		fmt.Printf("\x1b[36m[PTY Inbound (type %d)]:\x1b[0m\n%s\n", msgType, string(msg))
	}

	// Send Command
	fmt.Printf("\x1b[33m[PTY Outbound]: %s\x1b[0m\n", *cmd)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(*cmd)); err != nil {
		log.Fatalf("Failed to send command: %v", err)
	}

	// Read Output
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 2; i++ {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		fmt.Printf("\x1b[32m%s\x1b[0m", string(msg))
	}

	// Send Window Resize event
	fmt.Println("\n\x1b[35m[Sending Terminal Resize Event: 120x40]\x1b[0m")
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":120,"rows":40}`))

	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msg, err = conn.ReadMessage()
	if err == nil {
		fmt.Printf("\x1b[32m%s\x1b[0m", string(msg))
	}

	// Close cleanly
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"))
	fmt.Println("\n\x1b[32m[Console session cleanly detached]\x1b[0m")
}
