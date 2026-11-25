package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tscolari/worktool/internal/hooks"
)

// startNamed creates workspaces from clone without tmux.
func startNamed(t *testing.T, clone string, args map[string]StartOptions) {
	t.Helper()
	noTmux(t)
	t.Chdir(clone)
	for arg, opts := range args {
		require.NoError(t, RunWork([]string{arg}, opts, &strings.Builder{}, &strings.Builder{}), "RunWork %s", arg)
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
	require.NoError(t, RunList(&stdout, &strings.Builder{}))

	want := strings.Join([]string{
		listLine("A-a", "tester/A/a") + "  [remote: b1]",
		listLine("B-b", "tester/B/b") + "  [remote: b2]",
		listLine("C-c", "tester/C/c"),
	}, "\n") + "\n"
	require.Equal(t, want, stdout.String(), "list output")
	require.Equal(t, "A-a\nB-b\n", readFile(t, filepath.Join(logDir, "list")), "want one list hook run with A-a and B-b")
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
	require.NoError(t, RunList(&stdout, &stderr), "RunList must not fail when a list hook fails")

	want := listLine("A-a", "tester/A/a") + "\n" + listLine("C-c", "tester/C/c") + "\n"
	require.Equal(t, want, stdout.String(), "list output")
	require.Contains(t, stderr.String(), "warning:", "want a warning about the failed hook")
}
