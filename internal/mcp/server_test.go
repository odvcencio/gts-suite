package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestServerMalformedJSON(t *testing.T) {
	service := NewService(".", "")

	// Send garbage bytes with proper Content-Length framing.
	garbage := []byte("this is not json{{{")
	raw := "Content-Length: " + intToString(len(garbage)) + "\r\n\r\n" + string(garbage)

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, bytes.NewReader([]byte(raw)), output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", resp)
	}
	code := rpcErr["code"].(float64)
	if int(code) != -32700 {
		t.Fatalf("expected error code -32700 (Parse error), got %d", int(code))
	}
}

func TestServerEmptyMethod(t *testing.T) {
	service := NewService(".", "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "",
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", resp)
	}
	code := rpcErr["code"].(float64)
	if int(code) != -32600 {
		t.Fatalf("expected error code -32600 (Invalid Request), got %d", int(code))
	}
}

func TestServerUnknownMethod(t *testing.T) {
	service := NewService(".", "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "bogus/method",
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", resp)
	}
	code := rpcErr["code"].(float64)
	if int(code) != -32601 {
		t.Fatalf("expected error code -32601 (Method not found), got %d", int(code))
	}
	msg := rpcErr["message"].(string)
	if !strings.Contains(msg, "bogus/method") {
		t.Fatalf("expected error message to reference method name, got %q", msg)
	}
}

func TestServerUnknownTool(t *testing.T) {
	service := NewService(".", "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "nonexistent_tool",
		},
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	// Unknown tool returns a result with isError=true, not an RPC-level error.
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %#v", resp)
	}
	if isError, ok := result["isError"].(bool); !ok || !isError {
		t.Fatalf("expected isError=true, got %#v", result["isError"])
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content, got %#v", result["content"])
	}
	firstContent := content[0].(map[string]any)
	text := firstContent["text"].(string)
	if !strings.Contains(text, "nonexistent_tool") {
		t.Fatalf("expected error text to reference tool name, got %q", text)
	}
}

func TestServerMissingParams(t *testing.T) {
	service := NewService(".", "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		// No "params" field at all.
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	// decodeParams with empty raw returns nil error, so params.Name is "",
	// which triggers the "missing tool name" rpcError (-32602).
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", resp)
	}
	code := rpcErr["code"].(float64)
	if int(code) != -32602 {
		t.Fatalf("expected error code -32602 (Invalid params), got %d", int(code))
	}
}

func TestServerToolCallError(t *testing.T) {
	// gts_refs requires "name" argument. Calling it with an empty name on an
	// empty directory will exercise the tool's error path.
	service := NewService(t.TempDir(), "")

	requests := bytes.NewBuffer(nil)
	appendFramedJSON(t, requests, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "gts_grep",
			"arguments": map[string]any{
				// selector is required but intentionally invalid.
				"selector": "###INVALID###",
			},
		},
	})

	output := bytes.NewBuffer(nil)
	if err := RunStdio(service, requests, output, bytes.NewBuffer(nil)); err != nil {
		t.Fatalf("RunStdio returned error: %v", err)
	}

	resp, _ := decodeFramedJSON(t, output.Bytes())
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %#v", resp)
	}
	if isError, ok := result["isError"].(bool); !ok || !isError {
		t.Fatalf("expected isError=true for tool error, got %#v", result["isError"])
	}
	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected _meta map, got %T", result["_meta"])
	}
	if okValue, ok := meta["ok"].(bool); !ok || okValue {
		t.Fatalf("expected _meta.ok=false, got %#v", meta["ok"])
	}
}

func stringsReader(value string) *bytes.Reader {
	return bytes.NewReader([]byte(value))
}

func intToString(value int) string {
	return strconv.FormatInt(int64(value), 10)
}
