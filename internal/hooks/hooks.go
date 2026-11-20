// Package hooks runs a profile's hook executables. A profile is a directory
// of executable files named after the events worktool fires; worktool knows
// nothing about what a hook does.
package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

// Event names a hook. The hook file inside a profile directory has this name.
type Event string

const (
	PostStart      Event = "post-start"
	PreAttach      Event = "pre-attach"
	SessionCommand Event = "session-command"
	List           Event = "list"
	PreEnd         Event = "pre-end"
	PostEnd        Event = "post-end"
)

var (
	ErrInvalidName = errors.New("invalid profile name")
	ErrNotFound    = errors.New("profile not found")
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidName reports whether name is an acceptable profile name.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// Profile is a resolved profile directory.
type Profile struct {
	Name string
	Dir  string
}

// Open resolves profile name under configDir/profiles. It fails with
// ErrInvalidName or ErrNotFound (wrapped) so callers can tell them apart.
func Open(configDir, name string) (Profile, error) {
	if !ValidName(name) {
		return Profile{}, fmt.Errorf("%w %q: must match %s", ErrInvalidName, name, nameRE)
	}
	dir := filepath.Join(configDir, "profiles", name)
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Profile{}, fmt.Errorf("%w: %q (no directory %s)", ErrNotFound, name, dir)
		}
		return Profile{}, fmt.Errorf("stat profile %q: %w", name, err)
	}
	if !info.IsDir() {
		return Profile{}, fmt.Errorf("%w: %q (%s is not a directory)", ErrNotFound, name, dir)
	}
	return Profile{Name: name, Dir: dir}, nil
}

// Env is what a hook learns about the workspace. Empty fields are not
// exported. Extra carries event-specific variables.
type Env struct {
	Name   string
	Branch string
	Path   string
	Repo   string
	Extra  map[string]string
}

// Vars returns the WORK_* variables for env, in KEY=value form.
func (e Env) Vars(profile string) []string {
	vars := []string{"WORK_PROFILE=" + profile}
	for _, kv := range [][2]string{
		{"WORK_NAME", e.Name},
		{"WORK_BRANCH", e.Branch},
		{"WORK_PATH", e.Path},
		{"WORK_REPO", e.Repo},
	} {
		if kv[1] != "" {
			vars = append(vars, kv[0]+"="+kv[1])
		}
	}
	keys := make([]string, 0, len(e.Extra))
	for k := range e.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vars = append(vars, k+"="+e.Extra[k])
	}
	return vars
}

// dir is the hook's working directory: the worktree while it exists, the
// main repo otherwise (post-end, end's recovery path). Empty means the
// caller's own working directory.
func (e Env) dir() string {
	if e.Path != "" {
		if info, err := os.Stat(e.Path); err == nil && info.IsDir() {
			return e.Path
		}
	}
	return e.Repo
}

// ExitError is a hook that ran and failed.
type ExitError struct {
	Event   Event
	Profile string
	Err     error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s hook (profile %s) failed: %v", e.Event, e.Profile, e.Err)
}

func (e *ExitError) Unwrap() error { return e.Err }

// Run executes the hook for ev with the given stdio. A missing hook is a
// no-op; a hook file without an execute bit is an error.
//
// While the hook runs, worktool catches SIGINT and SIGQUIT instead of dying
// from them. Ctrl-C reaches the hook too (same process group); the hook
// decides what to do, and worktool handles its exit like any other failure,
// so a caller's cleanup (post-start's rollback) still happens.
func (p Profile) Run(ev Event, env Env, stdin io.Reader, stdout, stderr io.Writer) error {
	path, ok, err := p.lookup(ev)
	if err != nil || !ok {
		return err
	}
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(), env.Vars(p.Name)...)
	cmd.Dir = env.dir()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr

	held := make(chan os.Signal, 1)
	signal.Notify(held, os.Interrupt, syscall.SIGQUIT)
	defer signal.Stop(held)

	if err := cmd.Run(); err != nil {
		return &ExitError{Event: ev, Profile: p.Name, Err: err}
	}
	return nil
}

// Output executes the hook for ev and returns its stdout, unmodified. stdin
// is the terminal's. A missing hook returns "".
func (p Profile) Output(ev Event, env Env, stderr io.Writer) (string, error) {
	var stdout bytes.Buffer
	if err := p.Run(ev, env, os.Stdin, &stdout, stderr); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func (p Profile) lookup(ev Event) (string, bool, error) {
	path := filepath.Join(p.Dir, string(ev))
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat hook %s: %w", path, err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return "", false, fmt.Errorf("hook %s is not an executable file", path)
	}
	return path, true, nil
}

// FirstLine returns the first line of s, trimmed of surrounding whitespace.
func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
