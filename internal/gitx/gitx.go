// Package gitx wraps the subset of git commands that work/workend need.
// All operations shell out; the package never links libgit2 or go-git.
package gitx

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func InsideRepo(dir string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return false, nil
		}
		return false, fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func BranchExists(dir, branch string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return false, fmt.Errorf("git show-ref: %w", err)
}

func WorktreeAdd(dir, path, branch string) error {
	return run(dir, "git", "worktree", "add", "-b", branch, path)
}

func WorktreeRemove(dir, path string) error {
	return run(dir, "git", "worktree", "remove", "--force", path)
}

func BranchDelete(dir, branch string) error {
	return run(dir, "git", "branch", "-D", branch)
}

// CommonDir returns the path to the main repository's .git directory's
// parent (the main worktree). Useful so workend can run git ops outside
// the worktree it's about to remove.
func CommonDir(dir string) (string, error) {
	out, err := output(dir, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	return filepath.Dir(gitDir), nil
}

// BranchOfWorktree returns the branch checked out in the worktree at path.
// Errors if the worktree has a detached HEAD.
func BranchOfWorktree(path string) (string, error) {
	out, err := output(path, "git", "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return "", err
	}
	ref := strings.TrimSpace(out)
	if ref == "HEAD" {
		return "", fmt.Errorf("worktree at %s has a detached HEAD", path)
	}
	return strings.TrimPrefix(ref, "refs/heads/"), nil
}

// BranchOfWorktreeByPath looks up a worktree's branch from
// `git worktree list --porcelain` by matching the filesystem path. Unlike
// BranchOfWorktree, this does not require the worktree directory to exist.
func BranchOfWorktreeByPath(repoDir, worktreePath string) (string, error) {
	out, err := output(repoDir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}

	target := filepath.Clean(worktreePath)

	var curPath, curBranch string
	scanner := bufio.NewScanner(strings.NewReader(out))
	check := func() string {
		if filepath.Clean(curPath) == target && curBranch != "" {
			return strings.TrimPrefix(curBranch, "refs/heads/")
		}
		return ""
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if b := check(); b != "" {
				return b, nil
			}
			curPath, curBranch = "", ""
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			curPath = val
		case "branch":
			curBranch = val
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan worktree list: %w", err)
	}
	if b := check(); b != "" {
		return b, nil
	}
	return "", fmt.Errorf("worktree at %s not found in git worktree list", worktreePath)
}

// WorktreePrune removes stale worktree entries whose directories no longer
// exist on disk.
func WorktreePrune(dir string) error {
	return run(dir, "git", "worktree", "prune")
}

// BranchCheckedOutElsewhere returns the worktree path (other than exceptPath)
// where branch is currently checked out, or "" if nowhere else.
func BranchCheckedOutElsewhere(repoDir, branch, exceptPath string) (string, error) {
	out, err := output(repoDir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}

	exceptResolved, _ := filepath.EvalSymlinks(exceptPath)
	if exceptResolved == "" {
		exceptResolved = exceptPath
	}

	var curPath, curBranch string
	scanner := bufio.NewScanner(strings.NewReader(out))
	flush := func() string {
		defer func() { curPath, curBranch = "", "" }()
		if curBranch != "refs/heads/"+branch {
			return ""
		}
		resolved, _ := filepath.EvalSymlinks(curPath)
		if resolved == "" {
			resolved = curPath
		}
		if resolved == exceptResolved {
			return ""
		}
		return curPath
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if hit := flush(); hit != "" {
				return hit, nil
			}
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			curPath = val
		case "branch":
			curBranch = val
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan worktree list: %w", err)
	}
	if hit := flush(); hit != "" {
		return hit, nil
	}
	return "", nil
}

// HasUnmergedCommits reports whether branch has commits not present on its
// upstream. If branch has no upstream configured, returns (false, nil).
func HasUnmergedCommits(dir, branch string) (bool, error) {
	upstream, err := output(dir, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
	if err != nil {
		// No upstream is the common case for a freshly created branch;
		// git exits non-zero. Treat that as "no upstream, nothing to check".
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return false, nil
		}
		return false, err
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return false, nil
	}

	out, err := output(dir, "git", "rev-list", "--count", upstream+".."+branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// DefaultBranch returns the name of the repository's default branch by
// reading refs/remotes/origin/HEAD. Falls back to "main" or "master" if
// that ref is absent.
func DefaultBranch(dir string) (string, error) {
	out, err := output(dir, "git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		name := strings.TrimSpace(out)
		// Strip the "origin/" prefix if present.
		name = strings.TrimPrefix(name, "origin/")
		if name != "" {
			return name, nil
		}
	}
	for _, candidate := range []string{"main", "master"} {
		exists, err := BranchExists(dir, candidate)
		if err != nil {
			return "", err
		}
		if exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch")
}

// MergedBranches returns the names of local branches that are fully merged
// into base (e.g. "main"), excluding base itself.
func MergedBranches(dir, base string) ([]string, error) {
	out, err := output(dir, "git", "branch", "--merged", base)
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		// git branch output has a fixed 2-char prefix: "  ", "* " (current),
		// or "+ " (checked out in another worktree).
		name := strings.TrimSpace(line[2:])
		if name == "" || name == base {
			continue
		}
		branches = append(branches, name)
	}
	return branches, nil
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func output(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}
