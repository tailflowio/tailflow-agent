package action

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type ExecActionTestSuite struct {
	suite.Suite
}

func TestExecAction(t *testing.T) {
	suite.Run(t, new(ExecActionTestSuite))
}

func (s *ExecActionTestSuite) SetupTest() {}

func (s *ExecActionTestSuite) TestExecuteStringCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "echo hello",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("hello\n", outMap["stdout"])
	s.Equal(0, outMap["exit_code"])
}

func (s *ExecActionTestSuite) TestExecuteArrayCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": []any{"echo", "hello", "world"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("hello world\n", outMap["stdout"])
}

func (s *ExecActionTestSuite) TestValidateMissingCommand() {
	a := NewExecAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *ExecActionTestSuite) TestExecuteFailedCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "false",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ExecActionTestSuite) TestValidateInvalidCommandType() {
	a := NewExecAction()
	err := a.Validate(newTestContext(map[string]any{
		"command": 12345,
	}))
	s.Error(err)
	s.Contains(err.Error(), "string or array")
}

func (s *ExecActionTestSuite) TestExecuteStreaming() {
	a := NewExecAction()

	var mu sync.Mutex
	var logs []string

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"command": "echo streaming-test",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, msg)
		},
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Equal("streaming-test\n", outMap["stdout"])
	s.Equal(0, outMap["exit_code"])

	mu.Lock()
	defer mu.Unlock()
	s.Contains(logs, "streaming-test")
}

func (s *ExecActionTestSuite) TestExecuteEmptyCommandArray() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": []any{},
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "empty command")
}

func (s *ExecActionTestSuite) TestExecuteWithEnvAndDir() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "pwd",
		"dir":     "/tmp",
		"env": map[string]any{
			"MY_VAR": "hello",
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	// macOS /tmp is a symlink to /private/tmp
	s.Contains(outMap["stdout"].(string), "tmp")
	s.Equal(0, outMap["exit_code"])
}

func (s *ExecActionTestSuite) TestValidateOKString() {
	a := NewExecAction()
	err := a.Validate(newTestContext(map[string]any{"command": "echo hello"}))
	s.NoError(err)
}

func (s *ExecActionTestSuite) TestValidateOKArray() {
	a := NewExecAction()
	err := a.Validate(newTestContext(map[string]any{"command": []any{"echo", "hello"}}))
	s.NoError(err)
}

func (s *ExecActionTestSuite) TestStreamingWithStderr() {
	a := NewExecAction()

	var mu sync.Mutex
	var logs []string

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"command": []any{"sh", "-c", "echo out && echo err >&2"},
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, msg)
		},
	}

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	outMap := out.(map[string]any)
	s.Contains(outMap["stdout"].(string), "out")
	s.Contains(outMap["stderr"].(string), "err")
	s.Equal(0, outMap["exit_code"])

	mu.Lock()
	defer mu.Unlock()
	// Should have both stdout and stderr: lines
	s.GreaterOrEqual(len(logs), 2)
}

func (s *ExecActionTestSuite) TestExecuteEmptyStringCommand() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "   ",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "empty command")
}

func (s *ExecActionTestSuite) TestStreamingFailedCommand() {
	a := NewExecAction()

	var mu sync.Mutex
	var logs []string

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"command": "false",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, msg)
		},
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "exec")
}

func (s *ExecActionTestSuite) TestStreamingStartError() {
	a := NewExecAction()

	ctx := &ActionContext{
		Context: context.Background(),
		Config: map[string]any{
			"command": "/nonexistent/binary/xyz",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {},
	}

	_, err := a.Execute(ctx)
	s.Error(err)
	s.Contains(err.Error(), "exec")
}

func (s *ExecActionTestSuite) TestNonStreamingStartError() {
	a := NewExecAction()

	ctx := newTestContext(map[string]any{
		"command": "/nonexistent/binary/xyz",
	})

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ExecActionTestSuite) TestExecuteContextCancelSIGINT() {
	a := NewExecAction()

	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ctx := &ActionContext{
		Context: cancelCtx,
		Config: map[string]any{
			"command": "sleep 60",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ExecActionTestSuite) TestStreamingContextCancelSIGINT() {
	a := NewExecAction()

	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ctx := &ActionContext{
		Context: cancelCtx,
		Config: map[string]any{
			"command": "sleep 60",
		},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {},
	}

	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *ExecActionTestSuite) TestExecuteEnvNotMap() {
	a := NewExecAction()
	ctx := newTestContext(map[string]any{
		"command": "echo hello",
		"env":     "not-a-map",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Contains(out.(map[string]any)["stdout"].(string), "hello")
}

func (s *ExecActionTestSuite) TestStreamingStdoutPipeError() {
	a := &ExecAction{}

	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {},
	}

	// Create a command with Stdout already set so StdoutPipe returns an error.
	cmd := exec.Command("echo", "hello")
	cmd.Stdout = &bytes.Buffer{}

	_, err := a.executeStreaming(ctx, cmd)
	s.Error(err)
	s.Contains(err.Error(), "stdout pipe")
}

func (s *ExecActionTestSuite) TestStreamingStderrPipeError() {
	a := &ExecAction{}

	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {},
	}

	// Create a command with Stderr already set so StderrPipe returns an error.
	// Stdout is NOT set, so StdoutPipe succeeds but StderrPipe fails.
	cmd := exec.Command("echo", "hello")
	cmd.Stderr = &bytes.Buffer{}

	_, err := a.executeStreaming(ctx, cmd)
	s.Error(err)
	s.Contains(err.Error(), "stderr pipe")
}

// errReader yields data then returns a non-EOF error on the next Read call.
type errReader struct {
	data []byte
	sent bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if !r.sent && len(r.data) > 0 {
		r.sent = true
		n := copy(p, r.data)

		return n, nil
	}

	return 0, io.ErrUnexpectedEOF
}

func (s *ExecActionTestSuite) TestCaptureStreams_ScannerError() {
	var mu sync.Mutex
	var logs []string

	ctx := &ActionContext{
		Context: context.Background(),
		Config:  map[string]any{},
		ExecCtx: runtime.NewExecutionContext("test-exec", "test-wf", nil, nil),
		StepID:  "test-step",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmitLog: func(msg string) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, msg)
		},
	}

	// errReader triggers scanner.Err() != nil inside captureStreams.
	stdoutReader := &errReader{data: []byte("line1\n")}
	stderrReader := &errReader{}

	captureStreams(ctx, stdoutReader, stderrReader)

	mu.Lock()
	defer mu.Unlock()

	found := false

	for _, l := range logs {
		if strings.Contains(l, "stream error") {
			found = true

			break
		}
	}

	s.True(found, "expected a 'stream error' log entry from the errReader")
}
