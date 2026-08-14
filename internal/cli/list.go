package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/tmuxx"
)

// RunList prints all workspaces under WorktreeBase with their branch and tmux session status.
func RunList(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return sysErr("load config: %v", err)
	}

	entries, err := os.ReadDir(cfg.WorktreeBase)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(stdout, "no active workspaces")
			return nil
		}
		return sysErr("read worktree base: %v", err)
	}

	checkSession := tmuxx.Installed()
	found := false

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found = true
		name := e.Name()
		path := filepath.Join(cfg.WorktreeBase, name)

		branch, err := gitx.BranchOfWorktree(path)
		if err != nil {
			branch = "(unknown)"
		}

		if checkSession {
			has, err := tmuxx.HasSession(name)
			if err != nil {
				has = false
			}
			sessionLabel := "[no session]"
			if has {
				sessionLabel = "[session]"
			}
			fmt.Fprintf(stdout, "%-40s  %-60s  %s\n", name, branch, sessionLabel)
		} else {
			fmt.Fprintf(stdout, "%-40s  %s\n", name, branch)
		}
	}

	if !found {
		fmt.Fprintln(stdout, "no active workspaces")
	}

	return nil
}
