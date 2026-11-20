package hooks

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupProfile(t *testing.T, name string) (cfgDir string, p Profile) {
	t.Helper()
	cfgDir = t.TempDir()
	dir := filepath.Join(cfgDir, "profiles", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Open(cfgDir, name)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}
	return cfgDir, p
}

func writeHook(t *testing.T, p Profile, ev Event, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(p.Dir, string(ev))
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOpen(t *testing.T) {
	cfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "profiles", "remote"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Open(cfgDir, "remote")
	if err != nil {
		t.Fatalf("Open(remote): %v", err)
	}
	want := Profile{Name: "remote", Dir: filepath.Join(cfgDir, "profiles", "remote")}
	if got != want {
		t.Fatalf("Open(remote) = %+v, want %+v", got, want)
	}

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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Open(%q) error = %v, want %v", tt.profile, err, tt.wantErr)
			}
		})
	}
}

func TestRunMissingHookIsNoop(t *testing.T) {
	_, p := setupProfile(t, "remote")
	if err := p.Run(PostStart, Env{}, nil, nil, nil); err != nil {
		t.Fatalf("Run on missing hook: %v", err)
	}
	out, err := p.Output(SessionCommand, Env{}, nil)
	if err != nil || out != "" {
		t.Fatalf("Output on missing hook = %q, %v; want \"\", nil", out, err)
	}
}

func TestRunNotExecutable(t *testing.T) {
	_, p := setupProfile(t, "remote")
	path := writeHook(t, p, PostStart, "exit 0", 0o644)

	err := p.Run(PostStart, Env{}, nil, nil, nil)
	if err == nil {
		t.Fatal("Run on non-executable hook: want error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(path)) {
		t.Fatalf("error %q does not name the hook path %q", err, path)
	}
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
	if err := p.Run(PostStart, env, nil, &stdout, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "remote|T-1-x|p/T-1/x|" + path + "|" + repo + "|refs/remotes/origin/v18\n" + path + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunDirFallsBackToRepo(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PostEnd, "pwd -P", 0o755)

	repo := realpath(t, t.TempDir())
	env := Env{Path: filepath.Join(repo, "gone"), Repo: repo}

	out, err := p.Output(PostEnd, env, nil)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if out != repo+"\n" {
		t.Fatalf("cwd = %q, want %q", out, repo+"\n")
	}
}

func TestRunNonZero(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PreEnd, "exit 3", 0o755)

	err := p.Run(PreEnd, Env{}, nil, nil, nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want *ExitError", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("pre-end hook (profile remote) failed")) {
		t.Fatalf("error = %q, want it to name the event and profile", err)
	}
}

// Ctrl-C reaches the whole foreground process group. worktool must outlive
// it while a hook runs, so the hook's failure is handled (e.g. post-start's
// rollback) instead of worktool dying mid-command.
func TestRunSurvivesInterrupt(t *testing.T) {
	_, p := setupProfile(t, "remote")
	writeHook(t, p, PostStart, `kill -INT $PPID; sleep 0.2; exit 130`, 0o755)

	err := p.Run(PostStart, Env{}, nil, nil, nil)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want *ExitError from the interrupted hook", err)
	}
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
		if got := FirstLine(tt.in); got != tt.want {
			t.Errorf("FirstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
