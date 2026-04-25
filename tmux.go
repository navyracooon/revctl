package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func startTmuxSession(ctx context.Context, dir string, state *SessionState) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	refreshCmd := fmt.Sprintf("%q _action --session-dir %q refresh", self, dir)
	nextCmd := fmt.Sprintf("%q _action --session-dir %q next-file", self, dir)
	prevCmd := fmt.Sprintf("%q _action --session-dir %q prev-file", self, dir)
	filesCmd := fmt.Sprintf("%q _files --session-dir %q", self, dir)

	windowTarget, err := prepareTmuxTarget(ctx, state)
	if err != nil {
		return err
	}
	sessionGuard := fmt.Sprintf("#{==:#{session_name},%s}", state.SessionName)
	quitCmd := "kill-session -t " + shQuote(state.SessionName)
	if state.ReuseSession {
		quitCmd = "kill-window -t " + shQuote(state.WindowTarget)
	}

	filesPane, err := tmuxOutput(ctx, "send-keys", "-t", windowTarget, "C-c")
	if err != nil {
		return err
	}
	filesPane, err = tmuxOutput(ctx, "display-message", "-p", "-t", windowTarget, "#{pane_id}")
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, "respawn-pane", "-k", "-t", strings.TrimSpace(filesPane), "-c", state.RepoRoot, "sh", "-lc", filesCmd); err != nil {
		return err
	}

	viewerPane, err := tmuxOutput(ctx, "split-window", "-h", "-P", "-F", "#{pane_id}", "-t", windowTarget, "-c", state.RepoRoot, "sh", "-lc", "printf 'revctl diff\\n'; exec tail -f /dev/null")
	if err != nil {
		return err
	}

	state.Panes = PaneState{
		Files:  strings.TrimSpace(filesPane),
		Viewer: strings.TrimSpace(viewerPane),
	}

	for _, args := range [][]string{
		{"select-pane", "-t", state.Panes.Files},
		{"resize-pane", "-t", state.Panes.Files, "-x", "28"},
	} {
		if _, err := tmuxOutput(ctx, args...); err != nil {
			return err
		}
	}

	for _, bind := range [][]string{
		{"bind-key", "-n", "M-n", "if-shell", "-F", sessionGuard, "run-shell " + shQuote(nextCmd)},
		{"bind-key", "-n", "M-p", "if-shell", "-F", sessionGuard, "run-shell " + shQuote(prevCmd)},
		{"bind-key", "-n", "M-R", "if-shell", "-F", sessionGuard, "run-shell " + shQuote(refreshCmd)},
		{"bind-key", "-n", "M-a", "if-shell", "-F", sessionGuard, "select-pane -t " + shQuote(state.Panes.Viewer)},
		{"bind-key", "-n", "M-q", "if-shell", "-F", sessionGuard, quitCmd},
	} {
		if _, err := tmuxOutput(ctx, bind...); err != nil {
			return err
		}
	}
	return nil
}

func prepareTmuxTarget(ctx context.Context, state *SessionState) (string, error) {
	if os.Getenv("TMUX") == "" {
		if _, err := tmuxOutput(ctx, "new-session", "-d", "-s", state.SessionName, "-c", state.RepoRoot); err != nil {
			return "", err
		}
		state.WindowTarget = state.SessionName + ":0"
		return state.WindowTarget + ".0", nil
	}

	sessionName, err := currentTmuxSessionName(ctx)
	if err != nil {
		return "", err
	}
	state.SessionName = sessionName
	state.ReuseSession = true

	windowID, err := tmuxOutput(ctx, "new-window", "-P", "-F", "#{window_id}", "-t", state.SessionName, "-n", "revctl", "-c", state.RepoRoot)
	if err != nil {
		return "", err
	}
	state.WindowTarget = strings.TrimSpace(windowID)
	return state.WindowTarget + ".0", nil
}

func currentTmuxSessionName(ctx context.Context) (string, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", errors.New("failed to detect current tmux session")
	}
	return name, nil
}

func tmuxOutput(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func attachOrSwitchTmuxSession(ctx context.Context, sessionName string) error {
	if !hasTerminal(os.Stdin) || !hasTerminal(os.Stdout) {
		return nil
	}

	args := []string{"attach-session", "-t", sessionName}
	action := "attach"
	if os.Getenv("TMUX") != "" {
		args = []string{"switch-client", "-t", sessionName}
		action = "switch-client"
	}

	out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %w (%s)", action, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hasTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func renderSession(ctx context.Context, dir string, state *SessionState) error {
	return renderSessionWithOptions(ctx, dir, state, renderOptions{refreshFiles: true, refreshDiffs: true})
}

func renderSessionWithOptions(ctx context.Context, dir string, state *SessionState, opts renderOptions) error {
	if opts.refreshFiles && state.Panes.Files != "" {
		filesCmd, err := filesPaneCommand(dir)
		if err != nil {
			return err
		}
		if _, err := tmuxOutput(ctx, "respawn-pane", "-k", "-t", state.Panes.Files, "-c", state.RepoRoot, "sh", "-lc", filesCmd); err != nil {
			return err
		}
	}
	if opts.refreshDiffs && state.Panes.Viewer != "" {
		if _, err := tmuxOutput(ctx, "respawn-pane", "-k", "-t", state.Panes.Viewer, "sh", "-lc", viewerPaneCommand(state)); err != nil {
			return err
		}
	}
	return nil
}

func paneWidth(ctx context.Context, paneID string) (int, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", paneID, "#{pane_width}")
	if err != nil {
		return 0, err
	}
	var width int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &width); err != nil {
		return 0, fmt.Errorf("parse pane width: %w", err)
	}
	return width, nil
}

func paneHeight(ctx context.Context, paneID string) (int, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", paneID, "#{pane_height}")
	if err != nil {
		return 0, err
	}
	var height int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &height); err != nil {
		return 0, fmt.Errorf("parse pane height: %w", err)
	}
	return height, nil
}

func paneSize(ctx context.Context, paneID string, fallbackWidth, fallbackHeight int) (int, int) {
	width, err := paneWidth(ctx, paneID)
	if err != nil || width <= 0 {
		width = fallbackWidth
	}
	height, err := paneHeight(ctx, paneID)
	if err != nil || height <= 0 {
		height = fallbackHeight
	}
	return width, height
}

func filesPaneCommand(sessionDir string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return fmt.Sprintf("%q _files --session-dir %q", self, sessionDir), nil
}
