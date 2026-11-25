package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
)

func TestEndPreEndBlocks(t *testing.T) {
	tests := []struct {
		name      string
		force     bool
		wantForce string
	}{
		{name: "without --force", force: false, wantForce: "\n"},
		{name: "--force still asks the hook", force: true, wantForce: "1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := newClone(t)
			cfgDir, base := setupConfig(t)
			logDir := hookLog(t)
			writeHook(t, cfgDir, "remote", hooks.PreEnd, `echo "$WORK_FORCE" > "$HOOK_LOG/pre-end"; exit 1`)
			path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
			t.Chdir(path)

			err := RunWorkend(tt.force, false, &strings.Builder{}, &strings.Builder{})

			requireCode(t, err, 1)
			require.DirExists(t, path, "worktree removed although pre-end refused")
			ok, _ := gitx.BranchExists(clone, testBranch)
			require.True(t, ok, "branch deleted although pre-end refused")
			require.Equal(t, tt.wantForce, readFile(t, filepath.Join(logDir, "pre-end")), "pre-end WORK_FORCE")
		})
	}
}

func TestEndDryRun(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	logDir := hookLog(t)
	writeHook(t, cfgDir, "remote", hooks.PreEnd, `echo "$WORK_DRY_RUN" > "$HOOK_LOG/pre-end"`)
	writeHook(t, cfgDir, "remote", hooks.PostEnd, `echo ran > "$HOOK_LOG/post-end"`)
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	t.Chdir(path)

	var stdout strings.Builder
	require.NoError(t, RunWorkend(false, true, &stdout, &strings.Builder{}))

	require.Equal(t, "1\n", readFile(t, filepath.Join(logDir, "pre-end")), "pre-end WORK_DRY_RUN")
	require.DirExists(t, path, "dry run removed the worktree")
	require.NoFileExists(t, filepath.Join(logDir, "post-end"), "post-end ran during a dry run")
	require.Contains(t, stdout.String(), "would run post-end hook (profile remote)")
}

func TestEndPostEndAfterGitBeforeTmux(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	logDir := hookLog(t)
	writeHook(t, cfgDir, "remote", hooks.PostEnd,
		`echo "$WORK_PROFILE" > "$HOOK_LOG/post-end"; if [ -e "$WORK_PATH" ]; then echo present; else echo absent; fi >> "$HOOK_LOG/post-end"`)
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	log := fakeTmux(t, true)
	t.Setenv("FAKE_TMUX_WATCH", filepath.Join(logDir, "post-end"))
	t.Chdir(path)

	require.NoError(t, RunWorkend(false, false, &strings.Builder{}, &strings.Builder{}))

	ok, _ := gitx.BranchExists(clone, testBranch)
	require.False(t, ok, "branch not deleted")
	require.Equal(t, "remote\nabsent\n", readFile(t, filepath.Join(logDir, "post-end")),
		"post-end should see profile remote and the worktree already gone")
	calls := tmuxLog(t, log)
	require.NotEmpty(t, calls, "no tmux calls")
	require.Equal(t, "kill-session -t T-1-x watched=yes", calls[len(calls)-1], "want kill-session after post-end ran")
}

func TestEndPostEndFailureWarns(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PostEnd, "exit 1")
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	t.Chdir(path)

	var stderr strings.Builder
	require.NoError(t, RunWorkend(false, false, &strings.Builder{}, &stderr), "post-end failure must not fail end")
	require.Contains(t, stderr.String(), "warning:")
}

func TestEndProfileDirGone(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PreEnd, "exit 0")
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	require.NoError(t, os.RemoveAll(filepath.Join(cfgDir, "profiles", "remote")))
	t.Chdir(path)

	err := RunWorkend(true, false, &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	require.DirExists(t, path, "worktree removed although its profile can't be loaded")
}

// orphanWorkspace creates T-1-x plus a sibling (so end can still find the
// main repo), then removes and prunes T-1-x behind end's back, leaving the
// shell in a directory whose branch can't be resolved. It returns the path.
func orphanWorkspace(t *testing.T) string {
	t.Helper()
	clone := newClone(t)
	_, base := setupConfig(t)
	path := startWorkspace(t, clone, base, StartOptions{})
	require.NoError(t, RunWork([]string{"T-2/y"}, StartOptions{}, &strings.Builder{}, &strings.Builder{}), "RunWork sibling")
	t.Chdir(path)
	t.Setenv("PWD", path)
	git(t, clone, "worktree", "remove", "--force", path)
	git(t, clone, "worktree", "prune")
	return path
}

func TestEndRecoveryUnknownBranch(t *testing.T) {
	path := orphanWorkspace(t)
	// Profiles exist, so the orphan might have had one.
	writeHook(t, filepath.Dir(os.Getenv("WORK_CONFIG")), "remote", hooks.PreEnd, "exit 0")

	err := RunWorkend(false, false, &strings.Builder{}, &strings.Builder{})
	msg := requireCode(t, err, 1)
	require.Contains(t, msg, path, "error should name the stale path")

	require.NoError(t, RunWorkend(true, false, &strings.Builder{}, &strings.Builder{}), "--force on the recovery path")
}

// Without any profiles, no hook could have been skipped: the recovery path
// behaves as it did before profiles existed.
func TestEndRecoveryNoProfilesUnchanged(t *testing.T) {
	orphanWorkspace(t)

	require.NoError(t, RunWorkend(false, false, &strings.Builder{}, &strings.Builder{}))
}
