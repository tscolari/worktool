package hooks

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProfile(t *testing.T, name string) (cfgDir string, p Profile) {
	t.Helper()
	cfgDir = t.TempDir()
	dir := filepath.Join(cfgDir, "profiles", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p, err := Open(cfgDir, name)
	require.NoError(t, err, "Open(%q)", name)
	return cfgDir, p
}

func writeHook(t *testing.T, p Profile, ev Event, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(p.Dir, string(ev))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), mode))
	return path
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

func TestOpen(t *testing.T) {
	cfgDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "profiles", "remote"), 0o755))

	got, err := Open(cfgDir, "remote")
	require.NoError(t, err)
	require.Equal(t, Profile{Name: "remote", Dir: filepath.Join(cfgDir, "profiles", "remote")}, got)

	tests := []struct {
		name    string
		profile string
		wantErr error
	}{
		{name: "uppercase", profile: "Remote", wantErr: ErrInvalidName},
		{name: "leading dash", profile: "-x", wantErr: ErrInvalidName},
		{name: "empty", profile: "", wantErr: ErrInvalidName},
		{name: "path separator", profile: "a/b", wantErr: ErrInvalidName},
		{name: "dot dot", profile: "..", wantErr: ErrInvalidName},
		{name: "missing directory", profile: "ghost", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(cfgDir, tt.profile)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestRunMissingHookIsNoop(t *testing.T) {
	_, p := setupProfile(t, "remote")
	require.NoError(t, p.Run(PostStart, Env{}, nil, nil, nil))

	out, err := p.Output(SessionCommand, Env{}, nil)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestRunNotExecutable(t *testing.T) {
	_, p := setupProfile(t, "remote")
	path := writeHook(t, p, PostStart, "exit 0", 0o644)

	err := p.Run(PostStart, Env{}, nil, nil, nil)
	require.ErrorContains(t, err, path, "error should name the hook path")
}

func TestRunEnvAndDir(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PostStart,
		`printf '%s|%s|%s|%s|%s|%s\n' "$WORK_PROFILE" "$WORK_NAME" "$WORK_BRANCH" "$WORK_PATH" "$WORK_REPO" "$WORK_BASE_REF"; pwd -P`,
		0o755)

	path := realpath(t, t.TempDir())
	repo := realpath(t, t.TempDir())
	env := Env{
		Name:   "T-1-x",
		Branch: "p/T-1/x",
		Path:   path,
		Repo:   repo,
		Extra:  map[string]string{"WORK_BASE_REF": "refs/remotes/origin/v18"},
	}

	var stdout bytes.Buffer
	require.NoError(t, p.Run(PostStart, env, nil, &stdout, nil))
	want := "remote|T-1-x|p/T-1/x|" + path + "|" + repo + "|refs/remotes/origin/v18\n" + path + "\n"
	require.Equal(t, want, stdout.String())
}

func TestRunDirFallsBackToRepo(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PostEnd, "pwd -P", 0o755)

	repo := realpath(t, t.TempDir())
	env := Env{Path: filepath.Join(repo, "gone"), Repo: repo}

	out, err := p.Output(PostEnd, env, nil)
	require.NoError(t, err)
	require.Equal(t, repo+"\n", out, "hook cwd")
}

func TestRunNonZero(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PreEnd, "exit 3", 0o755)

	err := p.Run(PreEnd, Env{}, nil, nil, nil)
	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	require.ErrorContains(t, err, "pre-end hook (profile remote) failed", "error should name the event and profile")
}

// Ctrl-C reaches the whole foreground process group. worktool must outlive
// it while a hook runs, so the hook's failure is handled (e.g. post-start's
// rollback) instead of worktool dying mid-command.
func TestRunSurvivesInterrupt(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PostStart, `kill -INT $PPID; sleep 0.2; exit 130`, 0o755)

	err := p.Run(PostStart, Env{}, nil, nil, nil)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr, "want *ExitError from the interrupted hook")
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ssh host-a\n", "ssh host-a"},
		{"  a  \nb\n", "a"},
		{"\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FirstLine(tt.in), "FirstLine(%q)", tt.in)
	}
}
