package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
)

// profileKey is the branch config key (branch.<branch>.workProfile) that
// records which profile a workspace was started with.
const profileKey = "workProfile"

// openProfile resolves a profile named on the command line.
func openProfile(cfg *config.Config, name string) (*hooks.Profile, error) {
	p, err := hooks.Open(cfg.Dir, name)
	if err != nil {
		if errors.Is(err, hooks.ErrInvalidName) || errors.Is(err, hooks.ErrNotFound) {
			return nil, userErr("%v", err)
		}
		return nil, sysErr("%v", err)
	}
	return &p, nil
}

// workspaceProfile returns the profile recorded on branch, or nil when the
// workspace has none. A recorded profile that can no longer be loaded is an
// error: carrying on without its hooks would skip whatever they guard.
func workspaceProfile(cfg *config.Config, dir, branch string) (*hooks.Profile, error) {
	name, err := gitx.BranchConfig(dir, branch, profileKey)
	if err != nil {
		return nil, sysErr("read profile of %s: %v", branch, err)
	}
	if name == "" {
		return nil, nil
	}
	p, err := hooks.Open(cfg.Dir, name)
	if err != nil {
		if errors.Is(err, hooks.ErrInvalidName) || errors.Is(err, hooks.ErrNotFound) {
			return nil, userErr("branch %s was started with profile %q, which can't be loaded: %v", branch, name, err)
		}
		return nil, sysErr("%v", err)
	}
	return &p, nil
}

// runHook runs ev for p with the terminal's stdin. A nil profile is a no-op.
func runHook(p *hooks.Profile, ev hooks.Event, env hooks.Env, stdout, stderr io.Writer) error {
	if p == nil {
		return nil
	}
	if err := p.Run(ev, env, os.Stdin, stdout, stderr); err != nil {
		return userErr("%v", err)
	}
	return nil
}

// sessionCommand asks p's session-command hook for the tmux session's
// command. "" means tmux's default shell.
func sessionCommand(p *hooks.Profile, env hooks.Env, stderr io.Writer) (string, error) {
	if p == nil {
		return "", nil
	}
	out, err := p.Output(hooks.SessionCommand, env, stderr)
	if err != nil {
		return "", userErr("%v", err)
	}
	return hooks.FirstLine(out), nil
}

// profilesConfigured reports whether any profile directory exists under
// <config dir>/profiles. An unreadable profiles directory counts as
// configured: when in doubt, assume hooks might matter.
func profilesConfigured(cfg *config.Config) bool {
	entries, err := os.ReadDir(filepath.Join(cfg.Dir, "profiles"))
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}
