package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Commander runs an external command and returns its combined output.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecCommander runs real processes.
type ExecCommander struct{}

// Run implements Commander.
func (ExecCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// tmux wraps the tmux CLI. socket (-L) isolates tests from the user's server.
type tmux struct {
	bin    string
	socket string
	cmd    Commander
}

func (t tmux) run(ctx context.Context, args ...string) (string, error) {
	sub := args[0]
	if t.socket != "" {
		args = append([]string{"-L", t.socket}, args...)
	}
	out, err := t.cmd.Run(ctx, t.bin, args...)
	if err != nil {
		return string(out), fmt.Errorf("tmux %s: %w: %s", sub, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// ensureSession creates a detached session unless it already exists.
func (t tmux) ensureSession(ctx context.Context, session string) error {
	if _, err := t.run(ctx, "has-session", "-t", "="+session); err == nil {
		return nil
	}
	if _, err := t.run(ctx, "new-session", "-d", "-s", session, "-n", "desk"); err != nil {
		return fmt.Errorf("create tmux session %s: %w", session, err)
	}
	return nil
}

// windowState reports whether session:window exists and whether its pane
// process is still running.
func (t tmux) windowState(ctx context.Context, session, window string) (exists, alive bool, err error) {
	out, err := t.run(ctx, "list-windows", "-t", "="+session, "-F", "#{window_name} #{pane_dead}")
	if err != nil {
		return false, false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, dead, ok := strings.Cut(line, " ")
		if ok && name == window {
			return true, dead != "1", nil
		}
	}
	return false, false, nil
}

// newWindow starts command in a new detached window that stays open after
// the command exits, so the user can read the result when attached.
func (t tmux) newWindow(ctx context.Context, session, window, command string) error {
	_, err := t.run(ctx,
		"new-window", "-d", "-t", "="+session+":", "-n", window, command, ";",
		"set-option", "-w", "-t", "="+session+":"+window, "remain-on-exit", "on")
	return err
}

func (t tmux) killWindow(ctx context.Context, session, window string) error {
	_, err := t.run(ctx, "kill-window", "-t", "="+session+":"+window)
	return err
}

// panePID returns the pid of session:window's pane process, which tmux starts
// as a session leader (so it is also the process group id).
func (t tmux) panePID(ctx context.Context, session, window string) (int, error) {
	out, err := t.run(ctx, "display-message", "-p", "-t", "="+session+":"+window, "#{pane_pid}")
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parse pane pid %q: %w", strings.TrimSpace(out), err)
	}
	return pid, nil
}
