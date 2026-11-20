package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tscolari/worktool/internal/hooks"
)

func TestAttachPreAttachBothPaths(t *testing.T) {
	tests := []struct {
		name       string
		hasSession bool
	}{
		{name: "detached session still alive", hasSession: true},
		{name: "session gone after exit", hasSession: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := newClone(t)
			cfgDir, base := setupConfig(t)
			logDir := hookLog(t)
			writeHook(t, cfgDir, "remote", hooks.PreAttach, `echo "$WORK_NAME" >> "$HOOK_LOG/pre-attach"`)
			writeHook(t, cfgDir, "remote", hooks.SessionCommand, `echo ran > "$HOOK_LOG/session-command"; echo 'ssh host-a'`)
			path := startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
			log := fakeTmux(t, tt.hasSession)

			if err := RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{}); err != nil {
				t.Fatalf("RunAttach: %v", err)
			}

			if got := readFile(t, filepath.Join(logDir, "pre-attach")); got != "T-1-x\n" {
				t.Fatalf("pre-attach runs = %q, want exactly one for T-1-x", got)
			}
			calls := strings.Join(tmuxLog(t, log), "\n")
			if tt.hasSession {
				if !strings.Contains(calls, "switch-client -t T-1-x") {
					t.Fatalf("tmux calls %q: want switch-client to the live session", calls)
				}
				if exists(filepath.Join(logDir, "session-command")) {
					t.Fatal("session-command ran although the session already existed")
				}
				return
			}
			if want := "new-session -d -s T-1-x -c " + path + " ssh host-a"; !strings.Contains(calls, want) {
				t.Fatalf("tmux calls %q: want %q", calls, want)
			}
		})
	}
}

func TestAttachPreAttachFails(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PreAttach, "exit 1")
	startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	log := fakeTmux(t, true)

	err := RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{})

	requireCode(t, err, 1)
	if calls := tmuxLog(t, log); len(calls) != 0 {
		t.Fatalf("tmux touched after pre-attach failed: %q", calls)
	}
}

func TestAttachProfileDirGone(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PreAttach, "exit 0")
	startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	if err := os.RemoveAll(filepath.Join(cfgDir, "profiles", "remote")); err != nil {
		t.Fatal(err)
	}
	log := fakeTmux(t, true)

	err := RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{})

	msg := requireCode(t, err, 1)
	if !strings.Contains(msg, "remote") {
		t.Fatalf("error %q does not name the profile", msg)
	}
	if calls := tmuxLog(t, log); len(calls) != 0 {
		t.Fatalf("tmux touched although the profile is missing: %q", calls)
	}
}

func TestAttachDetachedNoProfile(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)
	path := startWorkspace(t, clone, base, StartOptions{})
	git(t, path, "checkout", "-q", "--detach")
	log := fakeTmux(t, false)

	if err := RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatalf("RunAttach on detached workspace: %v", err)
	}
	if calls := strings.Join(tmuxLog(t, log), "\n"); !strings.Contains(calls, "new-session -d -s T-1-x -c "+path) {
		t.Fatalf("tmux calls %q: want a new session", calls)
	}
}
