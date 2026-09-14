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

// RunWorkend executes the `work end` command.
func RunWorkend(force, dryRun bool, stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return sysErr("load config: %v", err)
	}

	cwd, err := resolveWorkingDir()
	if err != nil {
		return sysErr("getwd: %v", err)
	}

	cwdResolved := resolvePathBestEffort(cwd)
	baseResolved, err := filepath.EvalSymlinks(cfg.WorktreeBase)
	if err != nil {
		baseResolved = cfg.WorktreeBase
	}

	if filepath.Dir(cwdResolved) != baseResolved {
		return userErr("workend must be run from inside a workspace under %s (cwd is %s)", cfg.WorktreeBase, cwd)
	}
	name := filepath.Base(cwdResolved)

	cwdExists := dirExists(cwdResolved)

	var branch, repoDir string

	if cwdExists {
		// Normal path: worktree directory still on disk.
		branch, err = gitx.BranchOfWorktree(cwd)
		if err != nil {
			return sysErr("read worktree HEAD: %v", err)
		}
		repoDir, err = gitx.CommonDir(cwd)
		if err != nil {
			return sysErr("locate main repo: %v", err)
		}
	} else {
		// Recovery path: the worktree directory was already removed (e.g. by
		// a previous work end while the shell was inside the directory).
		// Find the main repo through a sibling worktree and look up the
		// branch from git's worktree metadata.
		repoDir, err = repoDirFromSiblings(cfg.WorktreeBase, cwdResolved)
		if err != nil {
			return sysErr("locate main repo (worktree directory missing): %v", err)
		}
		branch, err = gitx.BranchOfWorktreeByPath(repoDir, cwdResolved)
		if err != nil {
			// Worktree entry is gone too — nothing left to clean up in git.
			branch = ""
		}
	}

	if branch != "" {
		other, err := gitx.BranchCheckedOutElsewhere(repoDir, branch, cwd)
		if err != nil {
			return sysErr("check other worktrees: %v", err)
		}
		if other != "" {
			return userErr("branch %s is checked out at %s; remove that worktree first", branch, other)
		}

		if !force {
			unmerged, err := gitx.HasUnmergedCommits(repoDir, branch)
			if err != nil {
				return sysErr("check unmerged commits: %v", err)
			}
			if unmerged {
				return userErr("branch %s has unmerged commits vs upstream; pass --force to delete anyway", branch)
			}
		}
	}

	if dryRun {
		fmt.Fprintf(stdout, "would kill tmux session: %s\n", name)
		if cwdExists {
			fmt.Fprintf(stdout, "would remove worktree:   %s\n", cwd)
		} else {
			fmt.Fprintf(stdout, "would prune worktree:    %s (directory already removed)\n", cwd)
		}
		if branch != "" {
			fmt.Fprintf(stdout, "would delete branch:     %s\n", branch)
		}
		return nil
	}

	// Remove the worktree and branch before touching tmux. When workend is run
	// from inside the session it's about to kill, killing the session sends
	// SIGHUP to this process; doing the git cleanup first guarantees it
	// completes regardless.
	if cwdExists {
		if err := gitx.WorktreeRemove(repoDir, cwd); err != nil {
			return sysErr("remove worktree: %v", err)
		}
	} else {
		// Directory is already gone; prune stale worktree metadata.
		if err := gitx.WorktreePrune(repoDir); err != nil {
			return sysErr("prune worktrees: %v", err)
		}
	}

	if branch != "" {
		if err := gitx.BranchDelete(repoDir, branch); err != nil {
			return sysErr("delete branch: %v", err)
		}
	}

	// Kill the tmux session last. This may terminate the current process if
	// workend was launched from within that session, so nothing important
	// should follow it.
	if tmuxx.Installed() {
		has, err := tmuxx.HasSession(name)
		if err != nil {
			return sysErr("check tmux session: %v", err)
		}
		if has {
			if err := tmuxx.KillSession(name); err != nil {
				fmt.Fprintf(stderr, "warning: kill tmux session %s: %v\n", name, err)
			}
		}
	}

	return nil
}

// resolveWorkingDir returns the current working directory. When os.Getwd
// fails (e.g. the directory has been removed from disk), it falls back to
// the PWD environment variable which the shell keeps set.
func resolveWorkingDir() (string, error) {
	cwd, err := os.Getwd()
	if err == nil {
		return cwd, nil
	}
	if pwd := os.Getenv("PWD"); pwd != "" {
		return pwd, nil
	}
	return "", err
}

// resolvePathBestEffort resolves symlinks in path. When the final component
// does not exist (e.g. a deleted worktree directory), it resolves the parent
// and reattaches the base name so comparisons with the resolved WorktreeBase
// still work.
func resolvePathBestEffort(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err == nil {
		return filepath.Join(parent, filepath.Base(path))
	}
	return filepath.Clean(path)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// repoDirFromSiblings discovers the main repository directory by running
// git commands from a sibling worktree that still exists under base.
func repoDirFromSiblings(base, skipPath string) (string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("read worktree base %s: %w", base, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(base, e.Name())
		if p == skipPath {
			continue
		}
		dir, err := gitx.CommonDir(p)
		if err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no sibling worktree found under %s; cannot locate main repository", base)
}
