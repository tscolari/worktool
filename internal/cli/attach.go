package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/tmuxx"
)

// RunAttach reconnects to the named workspace, creating a tmux session if needed.
func RunAttach(name string, stderr io.Writer) error {
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

	if tmuxx.InsideTmux() {
		if err := tmuxx.NewSessionDetached(name, path); err != nil {
			return sysErr("create tmux session: %v", err)
		}
		if err := tmuxx.SwitchClient(name); err != nil {
			return sysErr("switch tmux client: %v", err)
		}
		return nil
	}

	if err := tmuxx.NewSessionAttached(name, path); err != nil {
		return sysErr("attach tmux session: %v", err)
	}
	return nil
}
