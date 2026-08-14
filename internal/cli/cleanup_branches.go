package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tscolari/worktool/internal/gitx"
)

// RunCleanupBranches lists merged branches and deletes them after confirmation.
func RunCleanupBranches(yes bool, stdin io.Reader, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return sysErr("getwd: %v", err)
	}

	inRepo, err := gitx.InsideRepo(cwd)
	if err != nil {
		return sysErr("check git repo: %v", err)
	}
	if !inRepo {
		return userErr("not inside a git repository")
	}

	repoDir, err := gitx.CommonDir(cwd)
	if err != nil {
		return sysErr("locate main repo: %v", err)
	}

	base, err := gitx.DefaultBranch(repoDir)
	if err != nil {
		return sysErr("detect default branch: %v", err)
	}

	branches, err := gitx.MergedBranches(repoDir, base)
	if err != nil {
		return sysErr("list merged branches: %v", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(stdout, "no merged branches to clean up")
		return nil
	}

	fmt.Fprintln(stdout, "merged branches:")
	for _, b := range branches {
		fmt.Fprintf(stdout, "  %s\n", b)
	}

	if !yes {
		fmt.Fprintf(stdout, "delete %d branch(es)? [y/N] ", len(branches))
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		if answer := strings.TrimSpace(line); answer != "y" && answer != "Y" {
			fmt.Fprintln(stdout, "aborted")
			return nil
		}
	}

	for _, b := range branches {
		if err := gitx.BranchDelete(repoDir, b); err != nil {
			fmt.Fprintf(stderr, "warning: delete %s: %v\n", b, err)
			continue
		}
		fmt.Fprintf(stdout, "deleted %s\n", b)
	}

	return nil
}
