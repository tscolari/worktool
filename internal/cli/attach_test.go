package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

			require.NoError(t, RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{}))

			require.Equal(t, "T-1-x\n", readFile(t, filepath.Join(logDir, "pre-attach")), "want exactly one pre-attach run for T-1-x")
			calls := strings.Join(tmuxLog(t, log), "\n")
			if tt.hasSession {
				require.Contains(t, calls, "switch-client -t T-1-x", "want switch-client to the live session")
				require.NoFileExists(t, filepath.Join(logDir, "session-command"), "session-command ran although the session already existed")
				return
			}
			require.Contains(t, calls, "new-session -d -s T-1-x -c "+path+" ssh host-a")
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
	require.Empty(t, tmuxLog(t, log), "tmux touched after pre-attach failed")
}

func TestAttachProfileDirGone(t *testing.T) {
	clone := newClone(t)
	cfgDir, base := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.PreAttach, "exit 0")
	startWorkspace(t, clone, base, StartOptions{Profile: "remote"})
	require.NoError(t, os.RemoveAll(filepath.Join(cfgDir, "profiles", "remote")))
	log := fakeTmux(t, true)

	err := RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{})

	msg := requireCode(t, err, 1)
	require.Contains(t, msg, "remote", "error should name the profile")
	require.Empty(t, tmuxLog(t, log), "tmux touched although the profile is missing")
}

func TestAttachDetachedNoProfile(t *testing.T) {
	clone := newClone(t)
	_, base := setupConfig(t)
	path := startWorkspace(t, clone, base, StartOptions{})
	git(t, path, "checkout", "-q", "--detach")
	log := fakeTmux(t, false)

	require.NoError(t, RunAttach("T-1-x", &strings.Builder{}, &strings.Builder{}), "RunAttach on detached workspace")
	require.Contains(t, strings.Join(tmuxLog(t, log), "\n"), "new-session -d -s T-1-x -c "+path, "want a new session")
}
