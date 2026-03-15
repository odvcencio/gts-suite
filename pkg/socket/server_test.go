package socket

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestSocketPath(t *testing.T) {
	p1 := SocketPath("/home/user/project")
	p2 := SocketPath("/home/user/other")
	if p1 == p2 {
		t.Error("different workspaces should have different socket paths")
	}
	if p1 == "" {
		t.Error("socket path should not be empty")
	}
}

func TestServerRoundTrip(t *testing.T) {
	srv := NewServer(t.TempDir(), slog.Default())
	srv.Handle("ping", func(params json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})
	srv.Handle("echo", func(params json.RawMessage) (any, error) {
		var p map[string]string
		json.Unmarshal(params, &p)
		return p, nil
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Connect
	conn, err := net.Dial("unix", srv.Path())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Send ping
	conn.Write([]byte(`{"method":"ping"}` + "\n"))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	var resp Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}

	// Send echo
	conn.Write([]byte(`{"method":"echo","params":{"msg":"hello"}}` + "\n"))
	if !scanner.Scan() {
		t.Fatal("no response for echo")
	}
	json.Unmarshal(scanner.Bytes(), &resp)
	result, _ := json.Marshal(resp.Result)
	if string(result) != `{"msg":"hello"}` {
		t.Errorf("echo result = %s", result)
	}
}

func TestServerUnknownMethod(t *testing.T) {
	srv := NewServer(t.TempDir(), slog.Default())
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("unix", srv.Path())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte(`{"method":"nonexistent"}` + "\n"))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("no response")
	}
	var resp Response
	json.Unmarshal(scanner.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error for unknown method")
	}
}
