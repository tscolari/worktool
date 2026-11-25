package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
)

const testBranch = "tester/T-1/x"

func TestStartNoProfileUnchanged(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)

	path := startWorkspace(t, clone, base, StartOptions{})

	require.DirExists(t, path)
	require.Equal(t, testBranch, git(t, path, "rev-parse", "--abbrev-ref", "HEAD"), "worktree branch")
	got, _ := gitx.BranchConfig(clone, testBranch, profileKey)
	require.Empty(t, got, "workProfile should be unset")
}

func TestStartFrom(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)

	path := startWorkspace(t, clone, base, StartOptions{From: "origin/branch/v18"})

	require.Equal(t, git(t, clone, "rev-parse", "origin/branch/v18"), git(t, path, "rev-parse", "HEAD"),
		"worktree HEAD should be origin/branch/v18")
	got, _ := gitx.BranchConfig(clone, testBranch, "merge")
	require.Empty(t, got, "branch should have no upstream")
}

func TestStartFromUnknownRef(t *testing.T) {
	for _, ref := range []string{"nope", "-x"} {
		t.Run(ref, func(t *testing.T) {
			clone := newClone(t)
			_, base := setupConfig(t)
			noTmux(t)
			t.Chdir(clone)

			err := RunWork([]string{"T-1/x"}, StartOptions{From: ref}, &strings.Builder{}, &strings.Builder{})

			requireCode(t, err, 1)
			ok, _ := gitx.BranchExists(clone, testBranch)
			require.False(t, ok, "branch created despite unknown --from")
			require.NoDirExists(t, filepath.Join(base, "T-1-x"), "worktree created despite unknown --from")
		})
	}
}

func TestStartUnknownProfile(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)
	noTmux(t)
	t.Chdir(clone)

	err := RunWork([]string{"T-1/x"}, StartOptions{Profile: "ghost"}, &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	ok, _ := gitx.BranchExists(clone, testBranch)
	require.False(t, ok, "branch created despite unknown profile")
	require.NoDirExists(t, filepath.Join(base, "T-1-x"), "worktree created despite unknown profile")
}

func TestStartPostStartEnv(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	logDir := hookLog(t)
	writeHook(t, cfgDir, "remote", hooks.PostStart,
		`echo "$WORK_BASE_REF $WORK_BASE_COMMIT $WORK_BRANCH" > "$HOOK_LOG/post-start"`)

	startWorkspace(t, clone, base, StartOptions{Profile: "remote", From: "origin/branch/v18"})

	want := "refs/remotes/origin/branch/v18 " + git(t, clone, "rev-parse", "origin/branch/v18") + " " + testBranch + "\n"
	require.Equal(t, want, readFile(t, filepath.Join(logDir, "post-start")), "post-start env")
	got, _ := gitx.BranchConfig(clone, testBranch, profileKey)
	require.Equal(t, "remote", got, "workProfile")
}

func TestStartPostStartRollback(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PostStart, "exit 1")
	noTmux(t)
	t.Chdir(clone)

	err := RunWork([]string{"T-1/x"}, StartOptions{Profile: "remote"}, &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	ok, _ := gitx.BranchExists(clone, testBranch)
	require.False(t, ok, "branch left behind after post-start failure")
	require.NoDirExists(t, filepath.Join(base, "T-1-x"), "worktree left behind after post-start failure")
	got, _ := gitx.BranchConfig(clone, testBranch, profileKey)
	require.Empty(t, got, "workProfile left behind after rollback")
}

func TestStartSessionCommand(t *testing.T) {
	tests := []struct {
		name     string
		hookBody string
		wantTail string
	}{
		{name: "first line used", hookBody: `printf 'ssh host-a\nignored\n'`, wantTail: " ssh host-a"},
		{name: "blank means default shell", hookBody: `printf '\n'`, wantTail: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := newClone(t)
			cfgDir, base := setupConfig(t)
			writeHook(t, cfgDir, "remote", hooks.SessionCommand, tt.hookBody)
			log := fakeTmux(t, false)
			t.Chdir(clone)

			require.NoError(t, RunWork([]string{"T-1/x"}, StartOptions{Profile: "remote"}, &strings.Builder{}, &strings.Builder{}))

			lines := tmuxLog(t, log)
			require.NotEmpty(t, lines, "no tmux calls")
			require.Equal(t, "new-session -d -s T-1-x -c "+filepath.Join(base, "T-1-x")+tt.wantTail, lines[0], "first tmux call")
		})
	}
}
