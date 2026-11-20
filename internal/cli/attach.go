package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
	"github.com/tscolari/worktool/internal/tmuxx"
)

// RunAttach reconnects to the named workspace, creating a tmux session if needed.
func RunAttach(name string, stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return sysErr("load config: %v", err)
	}

	path := filepath.Join(cfg.WorktreeBase, name)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return userErr("workspace %q not found under %s", name, cfg.WorktreeBase)
		}
		return sysErr("stat %s: %v", path, err)
	}

	profile, env, err := attachProfile(cfg, name, path, stderr)
	if err != nil {
		return err
	}
	// pre-attach runs on every attach, whether the session survived a detach
	// or is about to be recreated after an exit.
	if err := runHook(profile, hooks.PreAttach, env, stdout, stderr); err != nil {
		return err
	}

	if !tmuxx.Installed() {
		return userErr("tmux not found; cd into the workspace manually: cd %s", path)
	}

	has, err := tmuxx.HasSession(name)
	if err != nil {
		return sysErr("check tmux session: %v", err)
	}

	if has {
		if tmuxx.InsideTmux() {
			if err := tmuxx.SwitchClient(name); err != nil {
				return sysErr("switch tmux client: %v", err)
			}
			return nil
		}
		if err := tmuxx.AttachSession(name); err != nil {
			return sysErr("attach tmux session: %v", err)
		}
		return nil
	}

	fmt.Fprintf(stderr, "no tmux session for %s; creating one\n", name)

	command, err := sessionCommand(profile, env, stderr)
	if err != nil {
		return err
	}

	if tmuxx.InsideTmux() {
		if err := tmuxx.NewSessionDetached(name, path, command); err != nil {
			return sysErr("create tmux session: %v", err)
		}
		if err := tmuxx.SwitchClient(name); err != nil {
			return sysErr("switch tmux client: %v", err)
		}
		return nil
	}

	if err := tmuxx.NewSessionAttached(name, path, command); err != nil {
		return sysErr("attach tmux session: %v", err)
	}
	return nil
}

// attachProfile finds the workspace's recorded profile. A worktree with a
// detached HEAD has no branch to read it from; attach then carries on
// without hooks, as it did before profiles existed.
func attachProfile(cfg *config.Config, name, path string, stderr io.Writer) (*hooks.Profile, hooks.Env, error) {
	env := hooks.Env{Name: name, Path: path}
	branch, err := gitx.BranchOfWorktree(path)
	if err != nil {
		fmt.Fprintf(stderr, "warning: cannot read the branch of %s (%v); profile hooks skipped\n", path, err)
		return nil, env, nil
	}
	repoDir, err := gitx.CommonDir(path)
	if err != nil {
		return nil, env, sysErr("locate main repo: %v", err)
	}
	env.Branch, env.Repo = branch, repoDir
	profile, err := workspaceProfile(cfg, repoDir, branch)
	if err != nil {
		return nil, env, err
	}
	return profile, env, nil
}
