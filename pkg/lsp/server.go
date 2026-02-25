package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// HandlerFunc processes a JSON-RPC request and returns a result or error.
type HandlerFunc func(params json.RawMessage) (any, error)

// NotifyFunc processes a JSON-RPC notification (no response expected).
type NotifyFunc func(params json.RawMessage)

// Server implements the JSON-RPC 2.0 transport for LSP.
type Server struct {
	reader   *bufio.Reader
	writer   io.Writer
	log      io.Writer
	handlers map[string]HandlerFunc
	notifs   map[string]NotifyFunc
	outMu    sync.Mutex
}

func NewServer(in io.Reader, out io.Writer, log io.Writer) *Server {
	return &Server{
		reader:   bufio.NewReader(in),
		writer:   out,
		log:      log,
		handlers: make(map[string]HandlerFunc),
		notifs:   make(map[string]NotifyFunc),
	}
}

func (s *Server) Handle(method string, fn HandlerFunc) {
	s.handlers[method] = fn
}

func (s *Server) OnNotify(method string, fn NotifyFunc) {
	s.notifs[method] = fn
}

// Serve reads messages in a loop until EOF or shutdown.
func (s *Server) Serve() error {
	for {
		err := s.ServeOnce()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ServeOnce reads and handles a single message.
func (s *Server) ServeOnce() error {
	msg, err := readMessage(s.reader)
	if err != nil {
		return err
	}

	isNotification := len(msg.ID) == 0 || string(msg.ID) == "null"
	if isNotification {
		if fn, ok := s.notifs[msg.Method]; ok {
			fn(msg.Params)
		}
		return nil
	}

	fn, ok := s.handlers[msg.Method]
	if !ok {
		return s.sendError(msg.ID, -32601, "method not found: "+msg.Method)
	}

	result, handlerErr := fn(msg.Params)
	if handlerErr != nil {
		return s.sendError(msg.ID, -32603, handlerErr.Error())
	}
	return s.sendResult(msg.ID, result)
}

func (s *Server) sendResult(id json.RawMessage, result any) error {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return writeMessage(s.writer, resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string) error {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return writeMessage(s.writer, resp)
}

// Notify sends a server-initiated notification (e.g., diagnostics).
func (s *Server) Notify(method string, params any) error {
	msg := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{JSONRPC: "2.0", Method: method, Params: params}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return writeMessage(s.writer, msg)
}

// readMessage reads a Content-Length framed JSON-RPC message.
func readMessage(r io.Reader) (rpcMessage, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	var contentLen int
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return rpcMessage{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLen, _ = strconv.Atoi(val)
		}
	}
	if contentLen == 0 {
		return rpcMessage{}, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(br, body); err != nil {
		return rpcMessage{}, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return rpcMessage{}, err
	}
	return msg, nil
}

// writeMessage writes a Content-Length framed JSON-RPC message.
func writeMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}
