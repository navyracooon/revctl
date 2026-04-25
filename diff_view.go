package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func viewerPaneCommand(state *SessionState) string {
	entry := state.currentEntry()
	if state.Base == "" {
		return buildDiffPaneCommand(state, diffPaneSpec{
			left:  diffSide{ref: "HEAD", path: entry.Path, empty: entry.ChangeType == "A"},
			right: diffSide{path: filepath.Join(state.RepoRoot, entry.Path), useFilePath: true, empty: entry.ChangeType == "D" || !fileExists(filepath.Join(state.RepoRoot, entry.Path))},
		})
	}
	return buildDiffPaneCommand(state, diffPaneSpec{
		left:  diffSide{ref: state.Base, path: entry.Path, empty: entry.ChangeType == "A"},
		right: diffSide{ref: state.Head, path: entry.Path, empty: entry.ChangeType == "D"},
	})
}

func buildDiffPaneCommand(state *SessionState, spec diffPaneSpec) string {
	editor := shellJoin(diffEditorArgs(state.Tools.Editor))
	var b strings.Builder
	fmt.Fprintf(&b, "tmpdir=$(mktemp -d) || exit 1; ")
	fmt.Fprintf(&b, "trap 'rm -rf \"$tmpdir\"' EXIT INT TERM; ")
	fmt.Fprintf(&b, "left=\"$tmpdir\"/%s; ", shQuote(tempDiffFileName("left", spec.left.path)))
	fmt.Fprintf(&b, "right=\"$tmpdir\"/%s; ", shQuote(tempDiffFileName("right", spec.right.path)))
	writeDiffSide(&b, state.RepoRoot, "left", spec.left)
	writeDiffSide(&b, state.RepoRoot, "right", spec.right)
	if spec.right.useFilePath && !spec.right.empty {
		fmt.Fprintf(&b, "exec %s -d \"$left\" %s", editor, shQuote(spec.right.path))
		return b.String()
	}
	fmt.Fprintf(&b, "exec %s -d \"$left\" \"$right\"", editor)
	return b.String()
}

func writeDiffSide(b *strings.Builder, repoRoot, varName string, side diffSide) {
	if side.empty {
		fmt.Fprintf(b, ": >\"$%s\"; ", varName)
		return
	}
	if side.useFilePath {
		fmt.Fprintf(b, "cp %s \"$%s\" 2>/dev/null || : >\"$%s\"; ", shQuote(side.path), varName, varName)
		return
	}
	fmt.Fprintf(b, "git -C %s show %s >\"$%s\" 2>/dev/null || : >\"$%s\"; ",
		shQuote(repoRoot),
		shQuote(side.ref+":"+side.path),
		varName,
		varName,
	)
}

func tempDiffFileName(prefix, path string) string {
	baseName := filepath.Base(path)
	if baseName == "" {
		baseName = "buffer"
	}
	return prefix + "-" + baseName
}
