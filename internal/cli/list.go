package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tscolari/worktool/internal/config"
	"github.com/tscolari/worktool/internal/gitx"
	"github.com/tscolari/worktool/internal/hooks"
	"github.com/tscolari/worktool/internal/tmuxx"
)

type listEntry struct {
	name, branch, profile string
}

// RunList prints all workspaces under WorktreeBase with their branch and tmux
// session status, followed by any annotation their profile's list hook gives.
func RunList(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return sysErr("load config: %v", err)
	}

	entries, err := os.ReadDir(cfg.WorktreeBase)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(stdout, "no active workspaces")
			return nil
		}
		return sysErr("read worktree base: %v", err)
	}

	var workspaces []listEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(cfg.WorktreeBase, name)

		w := listEntry{name: name, branch: "(unknown)"}
		if branch, err := gitx.BranchOfWorktree(path); err == nil {
			w.branch = branch
			w.profile, _ = gitx.BranchConfig(path, branch, profileKey)
		}
		workspaces = append(workspaces, w)
	}

	if len(workspaces) == 0 {
		fmt.Fprintln(stdout, "no active workspaces")
		return nil
	}

	annotations := listAnnotations(cfg, workspaces, stderr)
	checkSession := tmuxx.Installed()

	for _, w := range workspaces {
		line := fmt.Sprintf("%-40s  %s", w.name, w.branch)
		if checkSession {
			has, err := tmuxx.HasSession(w.name)
			if err != nil {
				has = false
			}
			sessionLabel := "[no session]"
			if has {
				sessionLabel = "[session]"
			}
			line = fmt.Sprintf("%-40s  %-60s  %s", w.name, w.branch, sessionLabel)
		}
		if a := annotations[w.name]; a != "" {
			line += "  " + a
		}
		fmt.Fprintln(stdout, line)
	}

	return nil
}

// listAnnotations runs each profile's list hook once, with all of that
// profile's workspaces in WORK_NAMES, and collects its `name<TAB>annotation`
// lines. Names it wasn't asked about are ignored. A profile that can't be
// loaded or whose hook fails costs a warning, never the listing.
func listAnnotations(cfg *config.Config, workspaces []listEntry, stderr io.Writer) map[string]string {
	var order []string
	byProfile := map[string][]string{}
	for _, w := range workspaces {
		if w.profile == "" {
			continue
		}
		if _, seen := byProfile[w.profile]; !seen {
			order = append(order, w.profile)
		}
		byProfile[w.profile] = append(byProfile[w.profile], w.name)
	}

	annotations := map[string]string{}
	for _, name := range order {
		names := byProfile[name]
		p, err := hooks.Open(cfg.Dir, name)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %v\n", err)
			continue
		}
		out, err := p.Output(hooks.List, hooks.Env{Extra: map[string]string{"WORK_NAMES": strings.Join(names, "\n")}}, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "warning: %v\n", err)
			continue
		}
		asked := map[string]bool{}
		for _, n := range names {
			asked[n] = true
		}
		for _, line := range strings.Split(out, "\n") {
			ws, annotation, ok := strings.Cut(line, "\t")
			if ok && asked[ws] {
				annotations[ws] = strings.TrimSpace(annotation)
			}
		}
	}
	return annotations
}
