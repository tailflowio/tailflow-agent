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
	args := parseCommandArgs(ctx)
	if len(args) == 0 {
		return nil, errors.New("exec action: empty command")
	}

	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Cancel = func() error {
		return command.Process.Signal(syscall.SIGINT)
	}
	command.WaitDelay = 5 * time.Second

	applyExecOptions(command, ctx)

	if ctx.EmitLog != nil {
		return a.executeStreaming(ctx, command)
	}

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()

	output := buildExecOutput(stdout.String(), stderr.String(), command.ProcessState.ExitCode())

	if err != nil {
		return output, fmt.Errorf("exec: %w\nstderr: %s", err, stderr.String())
	}

	return output, nil
}

func parseCommandArgs(ctx *ActionContext) []string {
	switch cmd := ctx.Config["command"].(type) {
	case string:
		return strings.Fields(cmd)
	case []any:
		args := make([]string, 0, len(cmd))
		for _, v := range cmd {
			args = append(args, fmt.Sprintf("%v", v))
		}

		return args
	default:
		return nil
	}
}

func applyExecOptions(command *exec.Cmd, ctx *ActionContext) {
	if dir, ok := ctx.Config["dir"]; ok {
		command.Dir = fmt.Sprintf("%v", dir)
	}

	if envMap, ok := ctx.Config["env"]; ok {
		if envM, ok := envMap.(map[string]any); ok {
			for k, v := range envM {
				command.Env = append(command.Env, fmt.Sprintf("%s=%v", k, v))
			}
		}
	}
}

func buildExecOutput(stdout, stderr string, exitCode int) map[string]any {
	return map[string]any{
		"stdout":    stdout,
		"stderr":    stderr,
		"exit_code": exitCode,
	}
}

func (a *ExecAction) executeStreaming(ctx *ActionContext, command *exec.Cmd) (any, error) {
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("exec: stdout pipe: %w", err)
	}

	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("exec: stderr pipe: %w", err)
	}

	startErr := command.Start()
	if startErr != nil {
		return nil, fmt.Errorf("exec: start: %w", startErr)
	}

	stdoutStr, stderrStr := captureStreams(ctx, stdoutPipe, stderrPipe)

	waitErr := command.Wait()

	output := buildExecOutput(stdoutStr, stderrStr, command.ProcessState.ExitCode())

	if waitErr != nil {
		return output, fmt.Errorf("exec: %w\nstderr: %s", waitErr, stderrStr)
	}

	return output, nil
}

func captureStreams(ctx *ActionContext, stdoutPipe, stderrPipe io.Reader) (string, string) {
	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup

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

	return stdoutBuf.String(), stderrBuf.String()
}
