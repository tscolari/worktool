package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			if !exists(path) {
				t.Fatal("worktree removed although pre-end refused")
			}
			if ok, _ := gitx.BranchExists(clone, testBranch); !ok {
				t.Fatal("branch deleted although pre-end refused")
			}
			if got := readFile(t, filepath.Join(logDir, "pre-end")); got != tt.wantForce {
				t.Fatalf("pre-end saw WORK_FORCE=%q, want %q", got, tt.wantForce)
			}
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
	if err := RunWorkend(false, true, &stdout, &strings.Builder{}); err != nil {
		t.Fatalf("RunWorkend --dry-run: %v", err)
	}

	if got := readFile(t, filepath.Join(logDir, "pre-end")); got != "1\n" {
		t.Fatalf("pre-end saw WORK_DRY_RUN=%q, want 1", got)
	}
	if !exists(path) {
		t.Fatal("dry run removed the worktree")
	}
	if exists(filepath.Join(logDir, "post-end")) {
		t.Fatal("post-end ran during a dry run")
	}
	if !strings.Contains(stdout.String(), "would run post-end hook (profile remote)") {
		t.Fatalf("dry-run output %q does not mention post-end", stdout.String())
	}
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

	if err := RunWorkend(false, false, &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("RunWorkend: %v", err)
	}

	if ok, _ := gitx.BranchExists(clone, testBranch); ok {
		t.Fatal("branch not deleted")
	}
	if got := readFile(t, filepath.Join(logDir, "post-end")); got != "remote\nabsent\n" {
		t.Fatalf("post-end saw %q, want profile remote and the worktree already gone", got)
	}
	calls := tmuxLog(t, log)
	if last := calls[len(calls)-1]; last != "kill-session -t T-1-x watched=yes" {
		t.Fatalf("last tmux call = %q, want kill-session after post-end ran", last)
	}
}

func TestEndPostEndFailureWarns(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PostEnd, "exit 1")
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	t.Chdir(path)

	var stderr strings.Builder
	if err := RunWorkend(false, false, &strings.Builder{}, &stderr); err != nil {
		t.Fatalf("RunWorkend: %v (post-end failure must not fail end)", err)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("stderr %q: want a warning about post-end", stderr.String())
	}
}

func TestEndProfileDirGone(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PreEnd, "exit 0")
	path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	if err := os.RemoveAll(filepath.Join(cfgDir, "profiles", "remote")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(path)

	err := RunWorkend(true, false, &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	if !exists(path) {
		t.Fatal("worktree removed although its profile can't be loaded")
	}
}

// orphanWorkspace creates T-1-x plus a sibling (so end can still find the
// main repo), then removes and prunes T-1-x behind end's back, leaving the
// shell in a directory whose branch can't be resolved. It returns the path.
func orphanWorkspace(t *testing.T) string {
	t.Helper()
	clone := newClone(t)
	_, base := setupConfig(t)
	path := startWorkspace(t, clone, base, StartOptions{})
	if err := RunWork([]string{"T-2/y"}, StartOptions{}, &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("RunWork sibling: %v", err)
	}
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
	if !strings.Contains(msg, path) {
		t.Fatalf("error %q does not name the stale path %s", msg, path)
	}

	if err := RunWorkend(true, false, &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("RunWorkend --force on the recovery path: %v", err)
	}
}

// Without any profiles, no hook could have been skipped: the recovery path
// behaves as it did before profiles existed.
func TestEndRecoveryNoProfilesUnchanged(t *testing.T) {
	orphanWorkspace(t)

	if err := RunWorkend(false, false, &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("RunWorkend on the recovery path without profiles: %v", err)
	}
}
