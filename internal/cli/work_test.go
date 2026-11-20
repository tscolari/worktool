package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
)

const testBranch = "tester/T-1/x"

func TestStartNoProfileUnchanged(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)

	path := startWorkspace(t, clone, base, StartOptions{})

	if !exists(path) {
		t.Fatalf("worktree %s not created", path)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != testBranch {
		t.Fatalf("worktree branch = %q, want %q", got, testBranch)
	}
	if got, _ := gitx.BranchConfig(clone, testBranch, profileKey); got != "" {
		t.Fatalf("workProfile = %q, want unset", got)
	}
}

func TestStartFrom(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)

	path := startWorkspace(t, clone, base, StartOptions{From: "origin/branch/v18"})

	if got, want := git(t, path, "rev-parse", "HEAD"), git(t, clone, "rev-parse", "origin/branch/v18"); got != want {
		t.Fatalf("worktree HEAD = %s, want origin/branch/v18 %s", got, want)
	}
	if got, _ := gitx.BranchConfig(clone, testBranch, "merge"); got != "" {
		t.Fatalf("branch merge = %q, want no upstream", got)
	}
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
			if ok, _ := gitx.BranchExists(clone, testBranch); ok {
				t.Fatal("branch created despite unknown --from")
			}
			if exists(filepath.Join(base, "T-1-x")) {
				t.Fatal("worktree created despite unknown --from")
			}
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
	if ok, _ := gitx.BranchExists(clone, testBranch); ok {
		t.Fatal("branch created despite unknown profile")
	}
	if exists(filepath.Join(base, "T-1-x")) {
		t.Fatal("worktree created despite unknown profile")
	}
}

func TestStartPostStartEnv(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	logDir := hookLog(t)
	writeHook(t, cfgDir, "remote", hooks.PostStart,
		`echo "$WORK_BASE_REF $WORK_BASE_COMMIT $WORK_BRANCH" > "$HOOK_LOG/post-start"`)

	startWorkspace(t, clone, base, StartOptions{Profile: "remote", From: "origin/branch/v18"})

	want := "refs/remotes/origin/branch/v18 " + git(t, clone, "rev-parse", "origin/branch/v18") + " " + testBranch + "\n"
	if got := readFile(t, filepath.Join(logDir, "post-start")); got != want {
		t.Fatalf("post-start saw %q, want %q", got, want)
	}
	if got, _ := gitx.BranchConfig(clone, testBranch, profileKey); got != "remote" {
		t.Fatalf("workProfile = %q, want remote", got)
	}
}

func TestStartPostStartRollback(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PostStart, "exit 1")
	noTmux(t)
	t.Chdir(clone)

	err := RunWork([]string{"T-1/x"}, StartOptions{Profile: "remote"}, &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	if ok, _ := gitx.BranchExists(clone, testBranch); ok {
		t.Fatal("branch left behind after post-start failure")
	}
	if exists(filepath.Join(base, "T-1-x")) {
		t.Fatal("worktree left behind after post-start failure")
	}
	if got, _ := gitx.BranchConfig(clone, testBranch, profileKey); got != "" {
		t.Fatalf("workProfile = %q left behind after rollback", got)
	}
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

			if err := RunWork([]string{"T-1/x"}, StartOptions{Profile: "remote"}, &strings.Builder{}, &strings.Builder{}); err != nil {
				t.Fatalf("RunWork: %v", err)
			}

			want := "new-session -d -s T-1-x -c " + filepath.Join(base, "T-1-x") + tt.wantTail
			lines := tmuxLog(t, log)
			if len(lines) == 0 || lines[0] != want {
				t.Fatalf("tmux calls = %q, want first %q", lines, want)
			}
		})
	}
}
