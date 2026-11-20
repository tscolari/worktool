package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tscolari/worktool/internal/hooks"
)

// startNamed creates workspaces from clone without tmux.
func startNamed(t *testing.T, clone string, args map[string]StartOptions) {
	t.Helper()
	noTmux(t)
	t.Chdir(clone)
	for arg, opts := range args {
		if err := RunWork([]string{arg}, opts, &strings.Builder{}, &strings.Builder{}); err != nil {
			t.Fatalf("RunWork %s: %v", arg, err)
		}
	}
}

func listLine(name, branch string) string {
	return fmt.Sprintf("%-40s  %s", name, branch)
}

func TestListAnnotations(t *testing.T) {
	clone := newClone(t)
	cfgDir, _ := setupConfig(t)
	logDir := hookLog(t)
	writeHook(t, cfgDir, "remote", hooks.List,
		`printf '%s\n' "$WORK_NAMES" >> "$HOOK_LOG/list"; printf 'A-a\t[remote: b1]\nB-b\t[remote: b2]\nZZZ\tstray\n'`)
	startNamed(t, clone, map[string]StartOptions{
		"A/a": {Profile: "remote"},
		"B/b": {Profile: "remote"},
		"C/c": {},
	})

	var stdout strings.Builder
	if err := RunList(&stdout, &strings.Builder{}); err != nil {
		t.Fatalf("RunList: %v", err)
	}

	want := strings.Join([]string{
		listLine("A-a", "tester/A/a") + "  [remote: b1]",
		listLine("B-b", "tester/B/b") + "  [remote: b2]",
		listLine("C-c", "tester/C/c"),
	}, "\n") + "\n"
	if stdout.String() != want {
		t.Fatalf("list output:\n%s\nwant:\n%s", stdout.String(), want)
	}
	if got := readFile(t, filepath.Join(logDir, "list")); got != "A-a\nB-b\n" {
		t.Fatalf("list hook runs saw WORK_NAMES %q, want one run with A-a and B-b", got)
	}
}

func TestListHookFails(t *testing.T) {
	clone := newClone(t)
	cfgDir, _ := setupConfig(t)
	writeHook(t, cfgDir, "remote", hooks.List, "exit 1")
	startNamed(t, clone, map[string]StartOptions{
		"A/a": {Profile: "remote"},
		"C/c": {},
	})

	var stdout, stderr strings.Builder
	if err := RunList(&stdout, &stderr); err != nil {
		t.Fatalf("RunList must not fail when a list hook fails: %v", err)
	}

	want := listLine("A-a", "tester/A/a") + "\n" + listLine("C-c", "tester/C/c") + "\n"
	if stdout.String() != want {
		t.Fatalf("list output:\n%s\nwant:\n%s", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("stderr %q: want a warning about the failed hook", stderr.String())
	}
}
