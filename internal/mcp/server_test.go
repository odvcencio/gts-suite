package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReadFramedMessage(t *testing.T) {
	payload := `{"x":"hello"}`
	raw := "Content-Length: " + intToString(len(payload)) + "\r\n\r\n" + payload
	message, err := readFramedMessage(bufio.NewReader(stringsReader(raw)))
	if err != nil {
		t.Fatalf("readFramedMessage returned error: %v", err)
	}
	if string(message) != payload {
		t.Fatalf("unexpected message body %q", string(message))
	}
}

func TestServerInitializeAndToolsList(t *testing.T) {
	service := NewService(".", "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	first, rest := decodeFramedJSON(t, output.Bytes())
	if first["error"] != nil {
		t.Fatalf("unexpected initialize error response: %#v", first)
	}
	result1 := first["result"].(map[string]any)
	if result1["protocolVersion"] != protocolVersion {
		t.Fatalf("unexpected protocolVersion %v", result1["protocolVersion"])
	}

	second, _ := decodeFramedJSON(t, rest)
	if second["error"] != nil {
		t.Fatalf("unexpected tools/list error response: %#v", second)
	}
	result2 := second["result"].(map[string]any)
	tools, ok := result2["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("expected non-empty tools list, got %#v", result2["tools"])
	}
}

func TestServerToolsCallIncludesMeta(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "main.go")
	source := `package sample

func A() {}

func B() { A() }
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	service := NewService(tmpDir, "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "gts_refs",
			"arguments": map[string]any{
				"name": "A",
			},
		},
	})
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "missing_tool",
		},
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	first, rest := decodeFramedJSON(t, output.Bytes())
	firstResult, ok := first["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected first result map, got %T", first["result"])
	}
	firstMeta, ok := firstResult["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected first _meta map, got %T", firstResult["_meta"])
	}
	if firstMeta["tool"] != "gts_refs" {
		t.Fatalf("expected first _meta.tool=gts_refs, got %#v", firstMeta["tool"])
	}
	if okValue, ok := firstMeta["ok"].(bool); !ok || !okValue {
		t.Fatalf("expected first _meta.ok=true, got %#v", firstMeta["ok"])
	}
	if _, ok := firstMeta["duration_ms"].(float64); !ok {
		t.Fatalf("expected first _meta.duration_ms number, got %T", firstMeta["duration_ms"])
	}
	if isError, ok := firstResult["isError"].(bool); ok && isError {
		t.Fatalf("expected first result isError to be false, got %#v", firstResult["isError"])
	}

	second, _ := decodeFramedJSON(t, rest)
	secondResult, ok := second["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected second result map, got %T", second["result"])
	}
	secondMeta, ok := secondResult["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected second _meta map, got %T", secondResult["_meta"])
	}
	if secondMeta["tool"] != "missing_tool" {
		t.Fatalf("expected second _meta.tool=missing_tool, got %#v", secondMeta["tool"])
	}
	if okValue, ok := secondMeta["ok"].(bool); !ok || okValue {
		t.Fatalf("expected second _meta.ok=false, got %#v", secondMeta["ok"])
	}
	if _, ok := secondMeta["duration_ms"].(float64); !ok {
		t.Fatalf("expected second _meta.duration_ms number, got %T", secondMeta["duration_ms"])
	}
	if isError, ok := secondResult["isError"].(bool); !ok || !isError {
		t.Fatalf("expected second result isError=true, got %#v", secondResult["isError"])
	}
}

func appendFramedJSON(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	header := []byte("Content-Length: ")
	header = append(header, []byte(intToString(len(payload)))...)
	header = append(header, []byte("\r\n\r\n")...)
	buffer.Write(header)
	buffer.Write(payload)
}

func decodeFramedJSON(t *testing.T, data []byte) (map[string]any, []byte) {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(data))
	payload, err := readFramedMessage(reader)
	if err != nil {
		t.Fatalf("readFramedMessage failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll failed: %v", err)
	}
	return parsed, remaining
}

func stringsReader(value string) *bytes.Reader {
	return bytes.NewReader([]byte(value))
}

func intToString(value int) string {
	return strconv.FormatInt(int64(value), 10)
}
