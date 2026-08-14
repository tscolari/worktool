package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/tmuxx"
	"github.com/tscolari/worktool/internal/workspace"
)

const workUsage = "usage: work TICKET-NUM/kebab-description"

// RunWork executes the `work` command. stderr is used for warnings.
func RunWork(args []string, stderr io.Writer) error {
	if len(args) != 1 {
		return userErr("%s", workUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		return sysErr("load config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return sysErr("getwd: %v", err)
	}

	inRepo, err := gitx.InsideRepo(cwd)
	if err != nil {
		return sysErr("check git repo: %v", err)
	}
	if !inRepo {
		return userErr("not inside a git repository")
	}

	names, err := workspace.Parse(args[0], cfg.BranchPrefix, cfg.WorktreeBase)
	if err != nil {
		return userErr("%v", err)
	}

	exists, err := gitx.BranchExists(cwd, names.Branch)
	if err != nil {
		return sysErr("check branch: %v", err)
	}
	if exists {
		return userErr("branch already exists; cd into %s or use workend to clean up first", names.Path)
	}

	if _, err := os.Stat(names.Path); err == nil {
		return userErr("worktree path already exists; cd into %s or use workend to clean up first", names.Path)
	} else if !os.IsNotExist(err) {
		return sysErr("stat %s: %v", names.Path, err)
	}

	if err := gitx.WorktreeAdd(cwd, names.Path, names.Branch); err != nil {
		return sysErr("create worktree: %v", err)
	}

	if !tmuxx.Installed() {
		fmt.Fprintln(stderr, "warning: tmux not found; created worktree and branch but skipped session setup")
		return nil
	}

	if tmuxx.InsideTmux() {
		if err := tmuxx.NewSessionDetached(names.Session, names.Path); err != nil {
			return sysErr("create tmux session: %v", err)
		}
		if err := tmuxx.SwitchClient(names.Session); err != nil {
			return sysErr("switch tmux client: %v", err)
		}
		return nil
	}

	// Outside tmux: replace this process with `tmux new-session`.
	if err := tmuxx.NewSessionAttached(names.Session, names.Path); err != nil {
		return sysErr("attach tmux session: %v", err)
	}
	return nil
}
