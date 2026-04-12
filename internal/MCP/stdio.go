package mcpbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/logging"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type stdioSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID uint64
}

func newStdioSession(ctx context.Context, cfg ServerConfig) (*stdioSession, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("mcp stdio command is empty")
	}

	logging.Info("MCP stdio starting: label=%s command=%s args=%v", cfg.Label, cfg.Command, cfg.Args)

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go logStdioStderr(cfg.Label, stderr)

	session := &stdioSession{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}
	if err := session.initialize(ctx); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *stdioSession) initialize(ctx context.Context) error {
	logging.Info("MCP stdio initialize begin")
	var result map[string]interface{}
	if err := s.request(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "leiAgent",
			"version": "0.0.1",
		},
	}, &result); err != nil {
		return err
	}
	logging.Info("MCP stdio initialize success: result=%v", result)
	return s.notify("notifications/initialized", map[string]interface{}{})
}

func (s *stdioSession) request(ctx context.Context, method string, params interface{}, out *map[string]interface{}) error {
	id := strconv.FormatUint(atomic.AddUint64(&s.nextID, 1), 10)
	logging.Info("MCP stdio request send: method=%s id=%s params=%v", method, id, params)
	if err := s.send(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		return err
	}

	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("mcp stdio request timeout waiting for response: method=%s id=%s", method, id)
		default:
		}

		msg, err := s.readMessage()
		if err != nil {
			return err
		}
		logging.Info("MCP stdio message received: id=%s method=%s has_result=%t has_error=%t", msg.ID, msg.Method, len(msg.Result) > 0, msg.Error != nil)
		if msg.ID == "" {
			logging.Info("MCP stdio notification/event received: method=%s", msg.Method)
			continue
		}
		if msg.ID != id {
			logging.Info("MCP stdio skipping unmatched response: want=%s got=%s", id, msg.ID)
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("mcp error %d: %s", msg.Error.Code, msg.Error.Message)
		}
		if len(msg.Result) == 0 {
			*out = map[string]interface{}{}
			return nil
		}
		if err := json.Unmarshal(msg.Result, out); err != nil {
			return fmt.Errorf("invalid mcp stdio result: %w", err)
		}
		logging.Info("MCP stdio request success: method=%s id=%s result=%v", method, id, *out)
		return nil
	}
}

func (s *stdioSession) notify(method string, params interface{}) error {
	logging.Info("MCP stdio notify send: method=%s params=%v", method, params)
	return s.send(rpcEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (s *stdioSession) send(msg rpcEnvelope) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	logging.Info("MCP stdio body send: %s", string(data))
	payload := append(data, '\n')
	_, err = s.stdin.Write(payload)
	return err
}

func (s *stdioSession) readMessage() (*rpcEnvelope, error) {
	for {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		logging.Info("MCP stdio line raw: %s", line)
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			contentLength, err := s.readContentLengthBody(line)
			if err != nil {
				return nil, err
			}
			return s.decodeMessage(contentLength)
		}
		return s.decodeLineMessage(line)
	}
}

func (s *stdioSession) readContentLengthBody(firstHeader string) (int, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(firstHeader), "content-length:"))
	contentLength, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	for {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		logging.Info("MCP stdio header: %s", line)
	}
	if contentLength <= 0 {
		return 0, fmt.Errorf("invalid mcp message content length")
	}
	return contentLength, nil
}

func (s *stdioSession) decodeMessage(contentLength int) (*rpcEnvelope, error) {
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.stdout, body); err != nil {
		return nil, err
	}
	logging.Info("MCP stdio body raw: %s", string(body))

	var msg rpcEnvelope
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *stdioSession) decodeLineMessage(line string) (*rpcEnvelope, error) {
	var msg rpcEnvelope
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, fmt.Errorf("invalid ndjson mcp message: %w", err)
	}
	return &msg, nil
}

func (s *stdioSession) Close() {
	logging.Info("MCP stdio closing session")
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.cmd != nil {
		_, _ = s.cmd.Process.Wait()
	}
}

func logStdioStderr(label string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logging.Warn("MCP stdio stderr: label=%s msg=%s", label, line)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		logging.Warn("MCP stdio stderr read error: label=%s err=%v", label, err)
	}
}
