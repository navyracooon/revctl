package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type treeLine struct {
	entryIndex int
	label      string
}

func renderFilesView(state *SessionState, width, height int) string {
	state.normalizeCursor()
	lines := buildTreeLines(state.Entries, entryIndices(state.Entries))

	var b strings.Builder
	contentWidth := max(12, width-2)
	accent := "\x1b[38;2;122;162;255m"
	muted := "\x1b[38;2;133;148;166m"
	title := "\x1b[38;2;230;236;241m"
	selected := "\x1b[48;2;34;41;57m\x1b[38;2;244;246;248m"
	reset := "\x1b[0m"

	fmt.Fprintf(&b, "%srevctl%s\n", accent, reset)
	fmt.Fprintf(&b, "%s%s%s\n", title, fitText(state.currentEntry().Path, contentWidth), reset)
	fmt.Fprintf(&b, "%s%s%s\n", muted, fitText(fmt.Sprintf("%d / %d", state.CursorFile+1, len(state.Entries)), contentWidth), reset)
	fmt.Fprintln(&b)

	listHeight := visibleFilesCount(height, len(lines))
	start, end := visibleTreeWindow(lines, state.CursorFile, listHeight)
	if start > 0 {
		fmt.Fprintf(&b, "%s%s%s\n", muted, fitText("...", contentWidth), reset)
	}

	for _, line := range lines[start:end] {
		label := line.label
		if line.entryIndex == state.CurrentFile {
			label += "  *"
		}
		text := fitText(label, contentWidth-2)
		switch {
		case line.entryIndex == state.CursorFile:
			fmt.Fprintf(&b, "%s %s %s\n", selected, padRight(text, contentWidth-2), reset)
		case line.entryIndex >= 0:
			fmt.Fprintf(&b, "  %s\n", padRight(text, contentWidth-2))
		default:
			fmt.Fprintf(&b, "%s  %s%s\n", muted, padRight(text, contentWidth-2), reset)
		}
	}

	if end < len(lines) {
		fmt.Fprintf(&b, "%s%s%s\n", muted, fitText("...", contentWidth), reset)
	}
	if len(lines) == 0 {
		fmt.Fprintf(&b, "%s%s%s\n", muted, fitText("(no files)", contentWidth), reset)
	}

	for rendered := end - start; rendered < listHeight; rendered++ {
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "%s%s%s\n", muted, fitText("Up/Down move  Enter select", contentWidth), reset)
	return b.String()
}

func visibleFilesCount(height, total int) int {
	if total <= 0 {
		return 0
	}
	const reservedLines = 6
	return min(total, max(1, height-reservedLines))
}

func buildTreeLines(entries []ReviewEntry, filtered []int) []treeLine {
	lines := make([]treeLine, 0, len(filtered))
	var prevParts []string
	for _, idx := range filtered {
		parts := strings.Split(entries[idx].Path, "/")
		common := sharedDirPrefix(prevParts, parts)
		for depth := common; depth < len(parts)-1; depth++ {
			lines = append(lines, treeLine{
				entryIndex: -1,
				label:      strings.Repeat("  ", depth) + parts[depth] + "/",
			})
		}
		lines = append(lines, treeLine{
			entryIndex: idx,
			label:      strings.Repeat("  ", max(0, len(parts)-1)) + parts[len(parts)-1],
		})
		prevParts = parts
	}
	return lines
}

func sharedDirPrefix(prevParts, parts []string) int {
	limit := min(len(prevParts), len(parts)) - 1
	if limit <= 0 {
		return 0
	}
	n := 0
	for n < limit && prevParts[n] == parts[n] {
		n++
	}
	return n
}

func visibleTreeWindow(lines []treeLine, cursorFile, listHeight int) (int, int) {
	if len(lines) == 0 || listHeight <= 0 {
		return 0, 0
	}
	cursorPos := 0
	for i, line := range lines {
		if line.entryIndex == cursorFile {
			cursorPos = i
			break
		}
	}
	start := max(0, cursorPos-(listHeight/2))
	end := min(len(lines), start+listHeight)
	start = max(0, end-listHeight)
	return start, end
}

func ttyRawMode(ctx context.Context) (func(), error) {
	saved, err := exec.CommandContext(ctx, "stty", "-F", "/dev/tty", "-g").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("read tty state: %w (%s)", err, strings.TrimSpace(string(saved)))
	}
	original := strings.TrimSpace(string(saved))
	if out, err := exec.CommandContext(ctx, "stty", "-F", "/dev/tty", "raw", "-echo").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("set tty raw mode: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return func() {
		_, _ = exec.Command("stty", "-F", "/dev/tty", original).CombinedOutput()
	}, nil
}

func runFilesInteractive(ctx context.Context, sessionDir string) error {
	restoreTTY, err := ttyRawMode(ctx)
	if err != nil {
		return err
	}
	defer restoreTTY()

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open tty: %w", err)
	}
	defer tty.Close()

	state, err := loadState(sessionDir)
	if err != nil {
		return err
	}

	for {
		width, height := paneSize(ctx, state.Panes.Files, 28, 20)
		if err := renderFilesTTY(tty, state, width, height); err != nil {
			return err
		}
		input, err := readTTYKey(tty)
		if err != nil {
			return err
		}
		nextState, err := handleFilesInput(ctx, sessionDir, state, input)
		if err != nil {
			return err
		}
		state = nextState
	}
}

func renderFilesTTY(tty *os.File, state *SessionState, width, height int) error {
	if _, err := tty.WriteString(ttyFrame("\x1b[?25l\x1b[2J\x1b[H" + renderFilesView(state, width, height))); err != nil {
		return fmt.Errorf("render files: %w", err)
	}
	return nil
}

func readTTYKey(tty *os.File) ([]byte, error) {
	buf := make([]byte, 8)
	n, err := tty.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read tty input: %w", err)
	}
	return buf[:n], nil
}

func handleFilesInput(ctx context.Context, sessionDir string, state *SessionState, input []byte) (*SessionState, error) {
	switch {
	case bytes.Equal(input, []byte{27, 91, 65}), bytes.Equal(input, []byte("k")):
		state.moveCursor(-1)
	case bytes.Equal(input, []byte{27, 91, 66}), bytes.Equal(input, []byte("j")):
		state.moveCursor(1)
	case bytes.Equal(input, []byte{13}), bytes.Equal(input, []byte{10}):
		return runStateAction(ctx, sessionDir, state, renderOptions{refreshFiles: false, refreshDiffs: true}, func(s *SessionState) {
			s.selectCursor()
		})
	case bytes.Equal(input, []byte{3}):
	}
	return state, nil
}
