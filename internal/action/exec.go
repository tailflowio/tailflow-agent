//go:build !saas

package action

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ExecAction runs shell commands.
type ExecAction struct{}

func NewExecAction() Action { return &ExecAction{} }

func (a *ExecAction) Validate(ctx *ActionContext) error {
	cmd, ok := ctx.Config["command"]
	if !ok {
		return errors.New("exec action requires 'command' in config")
	}

	switch cmd.(type) {
	case string:
	case []any:
	default:
		return errors.New("exec action 'command' must be a string or array of strings")
	}

	return nil
}

func (a *ExecAction) Execute(ctx *ActionContext) (any, error) {
	var args []string

	switch cmd := ctx.Config["command"].(type) {
	case string:
		args = strings.Fields(cmd)
	case []any:
		for _, v := range cmd {
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	if len(args) == 0 {
		return nil, errors.New("exec action: empty command")
	}

	command := exec.CommandContext(ctx, args[0], args[1:]...)

	// Graceful shutdown: send SIGINT first, then SIGKILL after 5s grace period
	command.Cancel = func() error {
		return command.Process.Signal(syscall.SIGINT)
	}
	command.WaitDelay = 5 * time.Second

	// Set working directory
	if dir, ok := ctx.Config["dir"]; ok {
		command.Dir = fmt.Sprintf("%v", dir)
	}

	// Set environment variables
	if envMap, ok := ctx.Config["env"]; ok {
		if envM, ok := envMap.(map[string]any); ok {
			for k, v := range envM {
				command.Env = append(command.Env, fmt.Sprintf("%s=%v", k, v))
			}
		}
	}

	// When EmitLog is available, stream stdout/stderr line by line.
	if ctx.EmitLog != nil {
		return a.executeStreaming(ctx, command)
	}

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()

	output := map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": command.ProcessState.ExitCode(),
	}

	if err != nil {
		return output, fmt.Errorf("exec: %w\nstderr: %s", err, stderr.String())
	}

	return output, nil
}

// executeStreaming runs the command while emitting each output line as a live log.
func (a *ExecAction) executeStreaming(ctx *ActionContext, command *exec.Cmd) (any, error) {
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("exec: stdout pipe: %w", err)
	}

	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("exec: stderr pipe: %w", err)
	}

	if startErr := command.Start(); startErr != nil {
		return nil, fmt.Errorf("exec: start: %w", startErr)
	}

	var (
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
		wg        sync.WaitGroup
	)

	// Stream helper: scans lines from a reader, emits them, and captures the full output.
	streamLines := func(r io.Reader, buf *bytes.Buffer, prefix string) {
		defer wg.Done()

		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')

			if prefix != "" {
				ctx.EmitLog(prefix + line)
			} else {
				ctx.EmitLog(line)
			}
		}
	}

	wg.Add(2) //nolint:mnd // stdout + stderr

	go streamLines(stdoutPipe, &stdoutBuf, "")
	go streamLines(stderrPipe, &stderrBuf, "stderr: ")

	wg.Wait()

	waitErr := command.Wait()

	output := map[string]any{
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
		"exit_code": command.ProcessState.ExitCode(),
	}

	if waitErr != nil {
		return output, fmt.Errorf("exec: %w\nstderr: %s", waitErr, stderrBuf.String())
	}

	return output, nil
}
