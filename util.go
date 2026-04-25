package main

import (
	"os"
	"os/exec"
	"strings"
)

func detectEditor() string {
	for _, name := range []string{"nvim", "vim"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "nvim"
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func editorCommandArgs(command string) []string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func diffEditorArgs(command string) []string {
	args := editorCommandArgs(command)
	if len(args) == 0 {
		return nil
	}
	return append(args,
		"-c", "set laststatus=0 showtabline=0 winbar=",
		"-c", "set fillchars=vert:\\ ,fold:\\ ,eob:\\ ",
		"-c", "set diffopt+=algorithm:histogram,indent-heuristic,linematch:60",
		"-c", "set signcolumn=no foldcolumn=0 nowrap number",
		"-c", "set nocursorline nospell",
		"-c", "windo set scrollbind cursorbind",
		"-c", "wincmd =",
	)
}

func shQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-(width-1):])
}

func padRight(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func ttyFrame(text string) string {
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
