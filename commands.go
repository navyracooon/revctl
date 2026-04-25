package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runReview(ctx, args)
	}

	switch args[0] {
	case "review":
		return runReview(ctx, args[1:])
	case "_action":
		return runAction(ctx, args[1:])
	case "_render":
		return runRender(ctx, args[1:])
	case "_files":
		return runFiles(ctx, args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Println(`revctl: tmux-oriented Git review orchestrator

Usage:
  revctl [--base BASE] [--head HEAD] [--session-name NAME] [--no-attach]
`)
}

func runReview(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	base := fs.String("base", "", "git revision to diff from")
	head := fs.String("head", "HEAD", "git revision to diff to")
	sessionName := fs.String("session-name", "", "tmux session name")
	noAttach := fs.Bool("no-attach", false, "create tmux session without attaching")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux is required")
	}

	repoRoot, err := gitRoot(ctx)
	if err != nil {
		return err
	}

	diffRange, entries, err := loadReviewEntries(ctx, repoRoot, *base, *head)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("no diff entries found")
	}

	dir, err := os.MkdirTemp("", "revctl-*")
	if err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	name := *sessionName
	if name == "" {
		name = fmt.Sprintf("revctl-%d", time.Now().Unix())
	}

	state := &SessionState{
		SessionName: name,
		RepoRoot:    repoRoot,
		Base:        *base,
		Head:        *head,
		DiffRange:   diffRange,
		CurrentFile: 0,
		CursorFile:  0,
		Entries:     entries,
		UpdatedAt:   time.Now().Format(time.RFC3339),
		Tools: ToolState{
			Editor: detectEditor(),
		},
	}

	if err := saveState(dir, state); err != nil {
		return err
	}
	if err := startTmuxSession(ctx, dir, state); err != nil {
		return err
	}
	if err := saveState(dir, state); err != nil {
		return err
	}
	if err := renderSession(ctx, dir, state); err != nil {
		return err
	}

	fmt.Printf("session=%s\n", name)
	fmt.Printf("session_dir=%s\n", dir)
	fmt.Printf("attach: tmux attach -t %s\n", name)

	if !*noAttach {
		if err := attachOrSwitchTmuxSession(ctx, state.SessionName); err != nil {
			return err
		}
	}
	return nil
}

func runAction(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("action", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	sessionDir := fs.String("session-dir", "", "revctl session directory")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionDir == "" {
		return errors.New("--session-dir is required")
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("action name is required")
	}

	state, err := loadState(*sessionDir)
	if err != nil {
		return err
	}

	if err := applyAction(state, rest[0]); err != nil {
		return err
	}
	return commitState(ctx, *sessionDir, state, renderOptions{refreshFiles: true, refreshDiffs: true})
}

func runRender(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	sessionDir := fs.String("session-dir", "", "revctl session directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionDir == "" {
		return errors.New("--session-dir is required")
	}

	state, err := loadState(*sessionDir)
	if err != nil {
		return err
	}
	return renderSession(ctx, *sessionDir, state)
}

func runFiles(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("files", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	sessionDir := fs.String("session-dir", "", "revctl session directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sessionDir == "" {
		return errors.New("--session-dir is required")
	}

	return runFilesInteractive(ctx, *sessionDir)
}
