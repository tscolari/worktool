package gitx

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// isolateGit keeps the developer's global and system git config (signing,
// hooks, default branch) out of the tests.
func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "tester")
	t.Setenv("GIT_AUTHOR_EMAIL", "tester@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "tester")
	t.Setenv("GIT_COMMITTER_EMAIL", "tester@example.com")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s\n%s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// newClone creates an upstream repo with distinct commits on main and
// branch/v18, clones it and returns the clone's directory.
func newClone(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	up := filepath.Join(root, "up")
	git(t, root, "init", "-q", "-b", "main", up)
	git(t, up, "commit", "-q", "--allow-empty", "-m", "main-1")
	git(t, up, "checkout", "-q", "-b", "branch/v18")
	git(t, up, "commit", "-q", "--allow-empty", "-m", "v18-1")
	git(t, up, "checkout", "-q", "main")
	git(t, up, "commit", "-q", "--allow-empty", "-m", "main-2")
	clone := filepath.Join(root, "clone")
	git(t, root, "clone", "-q", up, clone)
	return clone
}

func TestWorktreeAddNoTrack(t *testing.T) {
	clone := newClone(t)
	wt := filepath.Join(t.TempDir(), "wt")

	require.NoError(t, WorktreeAdd(clone, wt, "tester/T-1/x", "origin/branch/v18"))

	merge, err := BranchConfig(clone, "tester/T-1/x", "merge")
	require.NoError(t, err)
	require.Empty(t, merge, "branch.tester/T-1/x.merge should be unset (--no-track)")
	require.Equal(t, git(t, clone, "rev-parse", "origin/branch/v18"), git(t, wt, "rev-parse", "HEAD"),
		"worktree HEAD should be origin/branch/v18")
}

func TestResolveBase(t *testing.T) {
	clone := newClone(t)
	v18 := git(t, clone, "rev-parse", "origin/branch/v18")
	head := git(t, clone, "rev-parse", "HEAD")

	tests := []struct {
		name        string
		ref         string
		detach      bool
		wantRef     string
		wantCommit  string
		wantUnknown bool
	}{
		{name: "remote-tracking branch", ref: "origin/branch/v18", wantRef: "refs/remotes/origin/branch/v18", wantCommit: v18},
		{name: "HEAD on a branch", ref: "HEAD", wantRef: "refs/heads/main", wantCommit: head},
		{name: "detached HEAD", ref: "HEAD", detach: true, wantRef: "", wantCommit: head},
		{name: "bare sha", ref: v18, wantRef: "", wantCommit: v18},
		{name: "unknown ref", ref: "nope", wantUnknown: true},
		{name: "option-like ref", ref: "-x", wantUnknown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.detach {
				git(t, clone, "checkout", "-q", "--detach")
				t.Cleanup(func() { git(t, clone, "checkout", "-q", "main") })
			}
			ref, commit, err := ResolveBase(clone, tt.ref)
			if tt.wantUnknown {
				require.ErrorIs(t, err, ErrUnknownRef)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantRef, ref)
			require.Equal(t, tt.wantCommit, commit)
		})
	}
}

func TestBranchConfigRoundTrip(t *testing.T) {
	clone := newClone(t)
	branch := "tester/T-1/x.y"
	git(t, clone, "branch", branch)

	require.NoError(t, SetBranchConfig(clone, branch, "workProfile", "remote"))
	got, err := BranchConfig(clone, branch, "workProfile")
	require.NoError(t, err)
	require.Equal(t, "remote", got)

	git(t, clone, "branch", "-D", branch)
	got, err = BranchConfig(clone, branch, "workProfile")
	require.NoError(t, err)
	require.Empty(t, got, "BranchConfig after branch -D")
}
