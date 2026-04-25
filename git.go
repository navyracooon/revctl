package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func gitRoot(ctx context.Context) (string, error) {
	out, err := gitOutput(ctx, "", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("git repository not found")
	}
	return strings.TrimSpace(out), nil
}

func loadReviewEntries(ctx context.Context, repoRoot, base, head string) (string, []ReviewEntry, error) {
	diffRange := head
	if strings.TrimSpace(base) != "" {
		diffRange = fmt.Sprintf("%s..%s", base, head)
	}

	args := []string{"diff", "--name-status"}
	args = append(args, diffRefs(base, head)...)

	out, err := gitOutput(ctx, repoRoot, args...)
	if err != nil {
		return "", nil, fmt.Errorf("git diff name-status: %w", err)
	}

	var entries []ReviewEntry
	for _, raw := range strings.Split(strings.TrimSpace(out), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		path := fields[len(fields)-1]
		firstLine, err := detectFirstChangedLine(ctx, repoRoot, base, head, path)
		if err != nil {
			firstLine = 1
		}
		entries = append(entries, ReviewEntry{
			Path:       path,
			ChangeType: string(fields[0][0]),
			FirstLine:  firstLine,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return diffRange, entries, nil
}

func diffRefs(base, head string) []string {
	if strings.TrimSpace(base) == "" {
		return []string{head}
	}
	return []string{base, head}
}

func detectFirstChangedLine(ctx context.Context, repoRoot, base, head, path string) (int, error) {
	args := []string{"diff", "--unified=0"}
	args = append(args, diffRefs(base, head)...)
	args = append(args, "--", path)

	out, err := gitOutput(ctx, repoRoot, args...)
	if err != nil {
		return 1, err
	}

	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		newStart, ok := parseHunkNewStart(line)
		if ok && newStart > 0 {
			return newStart, nil
		}
	}
	return 1, nil
}

func parseHunkNewStart(header string) (int, bool) {
	parts := strings.Split(header, " ")
	for _, part := range parts {
		if !strings.HasPrefix(part, "+") {
			continue
		}
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(part, "+"), "@@"))
		if part == "" {
			return 1, true
		}
		if idx := strings.Index(part, ","); idx >= 0 {
			part = part[:idx]
		}
		var n int
		if _, err := fmt.Sscanf(part, "%d", &n); err == nil {
			if n <= 0 {
				return 1, true
			}
			return n, true
		}
	}
	return 0, false
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
