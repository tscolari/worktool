package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
	"github.com/tscolari/worktool/internal/tmuxx"
	"github.com/tscolari/worktool/internal/workspace"
)

const workUsage = "usage: work TICKET-NUM/kebab-description"

// StartOptions are the optional flags of `work start`.
type StartOptions struct {
	// Profile names a profile under <config dir>/profiles whose hooks run
	// for this workspace from now on. Empty means no hooks.
	Profile string
	// From is the ref the branch starts at. Empty means HEAD.
	From string
}

// RunWork executes the `work start` command. Hook output goes to stdout and
// stderr; stderr is also used for warnings.
func RunWork(args []string, opts StartOptions, stdout, stderr io.Writer) error {
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

	var profile *hooks.Profile
	if opts.Profile != "" {
		if profile, err = openProfile(cfg, opts.Profile); err != nil {
			return err
		}
	}

	from := opts.From
	if from == "" {
		from = "HEAD"
	}
	baseRef, baseCommit, err := gitx.ResolveBase(cwd, from)
	if err != nil {
		if errors.Is(err, gitx.ErrUnknownRef) {
			return userErr("cannot start from %q: %v", from, err)
		}
		return sysErr("resolve %s: %v", from, err)
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

	repoDir, err := gitx.CommonDir(cwd)
	if err != nil {
		return sysErr("locate main repo: %v", err)
	}

	// Start at the resolved commit, so the branch is exactly what the hooks
	// are told in WORK_BASE_COMMIT.
	if err := gitx.WorktreeAdd(cwd, names.Path, names.Branch, baseCommit); err != nil {
		return sysErr("create worktree: %v", err)
	}

	env := hooks.Env{Name: names.Workspace, Branch: names.Branch, Path: names.Path, Repo: repoDir}
	if profile != nil {
		if err := gitx.SetBranchConfig(repoDir, names.Branch, profileKey, profile.Name); err != nil {
			return rollbackStart(repoDir, names, sysErr("record profile: %v", err))
		}
		startEnv := env
		startEnv.Extra = map[string]string{"WORK_BASE_REF": baseRef, "WORK_BASE_COMMIT": baseCommit}
		if err := runHook(profile, hooks.PostStart, startEnv, stdout, stderr); err != nil {
			return rollbackStart(repoDir, names, err)
		}
	}

	if !tmuxx.Installed() {
		fmt.Fprintln(stderr, "warning: tmux not found; created worktree and branch but skipped session setup")
		return nil
	}

	command, err := sessionCommand(profile, env, stderr)
	if err != nil {
		return err
	}

	if tmuxx.InsideTmux() {
		if err := tmuxx.NewSessionDetached(names.Session, names.Path, command); err != nil {
			return sysErr("create tmux session: %v", err)
		}
		if err := tmuxx.SwitchClient(names.Session); err != nil {
			return sysErr("switch tmux client: %v", err)
		}
		return nil
	}

	// Outside tmux: replace this process with `tmux new-session`.
	if err := tmuxx.NewSessionAttached(names.Session, names.Path, command); err != nil {
		return sysErr("attach tmux session: %v", err)
	}
	return nil
}

// rollbackStart removes the worktree and branch `start` just created (the
// branch's config, including the profile record, goes with the branch) and
// returns cause with the rollback's outcome appended.
func rollbackStart(repoDir string, names workspace.Names, cause error) error {
	code := 2
	var ce *Error
	if errors.As(cause, &ce) {
		code = ce.Code
	}
	msg := cause.Error() + "; workspace rolled back"
	if err := gitx.WorktreeRemove(repoDir, names.Path); err != nil {
		msg = fmt.Sprintf("%v; rollback failed, remove %s by hand: %v", cause, names.Path, err)
	} else if err := gitx.BranchDelete(repoDir, names.Branch); err != nil {
		msg = fmt.Sprintf("%v; rollback failed, delete branch %s by hand: %v", cause, names.Branch, err)
	}
	return &Error{Code: code, Msg: msg}
}
