package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tscolari/worktool/internal/hooks"
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

// tempDir is t.TempDir with symlinks resolved (macOS /var → /private/var),
// so paths compare equal to what git and the shell report.
func tempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}

// newClone creates an upstream repo with distinct commits on main and
// branch/v18, clones it and returns the clone's directory.
func newClone(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	root := tempDir(t)
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

// setupConfig writes a config with a temporary worktree_base and
// branch_prefix=tester, and points WORK_CONFIG at it.
func setupConfig(t *testing.T) (cfgDir, base string) {
	t.Helper()
	cfgDir = tempDir(t)
	base = filepath.Join(tempDir(t), "wt")
	cfg := fmt.Sprintf("worktree_base=%s\nbranch_prefix=tester\n", base)
	path := filepath.Join(cfgDir, "config")
	require.NoError(t, os.WriteFile(path, []byte(cfg), 0o644))
	t.Setenv("WORK_CONFIG", path)
	return cfgDir, base
}

// writeHook installs an executable sh hook for ev in profile.
func writeHook(t *testing.T, cfgDir, profile string, ev hooks.Event, body string) {
	t.Helper()
	dir := filepath.Join(cfgDir, "profiles", profile)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, string(ev)), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
}

// hookLog creates a directory hooks can write into, exported as $HOOK_LOG.
func hookLog(t *testing.T) string {
	t.Helper()
	dir := tempDir(t)
	t.Setenv("HOOK_LOG", dir)
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fakeTmux puts a tmux stand-in first on PATH and sets TMUX, so commands
// take the "inside tmux" path (no exec). Every call appends its arguments as
// one line to the returned log; has-session exits 0 when hasSession is set.
// When FAKE_TMUX_WATCH names a file, each line also records whether that file
// existed at the time of the call.
func fakeTmux(t *testing.T, hasSession bool) string {
	t.Helper()
	bin := tempDir(t)
	log := filepath.Join(tempDir(t), "tmux.log")
	code := 1
	if hasSession {
		code = 0
	}
	script := fmt.Sprintf(`#!/bin/sh
extra=""
if [ -n "$FAKE_TMUX_WATCH" ]; then
  if [ -e "$FAKE_TMUX_WATCH" ]; then extra=" watched=yes"; else extra=" watched=no"; fi
fi
echo "$*$extra" >> %q
if [ "$1" = "has-session" ]; then exit %d; fi
exit 0
`, log, code)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", "fake")
	return log
}

// noTmux leaves only git on PATH, so tmux counts as not installed.
func noTmux(t *testing.T) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err)
	bin := tempDir(t)
	require.NoError(t, os.Symlink(gitPath, filepath.Join(bin, "git")))
	t.Setenv("PATH", bin)
	t.Setenv("TMUX", "")
}

func tmuxLog(t *testing.T, path string) []string {
	t.Helper()
	if !exists(path) {
		return nil
	}
	return strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
}

// requireCode fails the test unless err is a *cli.Error with code, and
// returns its message.
func requireCode(t *testing.T, err error, code int) string {
	t.Helper()
	var ce *Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, code, ce.Code, "error code (msg %q)", ce.Msg)
	return ce.Msg
}

// startWorkspace creates workspace T-1-x from clone with no tmux, using the
// given options, and returns its path.
func startWorkspace(t *testing.T, clone, base string, opts StartOptions) string {
	t.Helper()
	noTmux(t)
	t.Chdir(clone)
	require.NoError(t, RunWork([]string{"T-1/x"}, opts, &strings.Builder{}, &strings.Builder{}))
	return filepath.Join(base, "T-1-x")
}
