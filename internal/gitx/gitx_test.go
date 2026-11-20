package gitx

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
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

	if err := WorktreeAdd(clone, wt, "tester/T-1/x", "origin/branch/v18"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	merge, err := BranchConfig(clone, "tester/T-1/x", "merge")
	if err != nil {
		t.Fatalf("BranchConfig: %v", err)
	}
	if merge != "" {
		t.Fatalf("branch.tester/T-1/x.merge = %q, want unset (--no-track)", merge)
	}
	if got, want := git(t, wt, "rev-parse", "HEAD"), git(t, clone, "rev-parse", "origin/branch/v18"); got != want {
		t.Fatalf("worktree HEAD = %s, want origin/branch/v18 %s", got, want)
	}
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
				if !errors.Is(err, ErrUnknownRef) {
					t.Fatalf("ResolveBase(%q) error = %v, want ErrUnknownRef", tt.ref, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveBase(%q): %v", tt.ref, err)
			}
			if ref != tt.wantRef || commit != tt.wantCommit {
				t.Fatalf("ResolveBase(%q) = %q, %q; want %q, %q", tt.ref, ref, commit, tt.wantRef, tt.wantCommit)
			}
		})
	}
}

func TestBranchConfigRoundTrip(t *testing.T) {
	clone := newClone(t)
	branch := "tester/T-1/x.y"
	git(t, clone, "branch", branch)

	if err := SetBranchConfig(clone, branch, "workProfile", "remote"); err != nil {
		t.Fatalf("SetBranchConfig: %v", err)
	}
	got, err := BranchConfig(clone, branch, "workProfile")
	if err != nil || got != "remote" {
		t.Fatalf("BranchConfig = %q, %v; want remote, nil", got, err)
	}

	git(t, clone, "branch", "-D", branch)
	got, err = BranchConfig(clone, branch, "workProfile")
	if err != nil || got != "" {
		t.Fatalf("BranchConfig after branch -D = %q, %v; want \"\", nil", got, err)
	}
}
