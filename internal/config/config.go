// Package config loads the work/workend configuration file and applies
// defaults for the worktree base directory and branch prefix.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	WorktreeBase string
	BranchPrefix string
	// Dir is the directory holding the config file (whether or not the file
	// exists). Profiles live under Dir/profiles.
	Dir string
}

func Load() (*Config, error) {
	path := os.Getenv("WORK_CONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".config", "work", "config")
	}

	cfg := &Config{Dir: filepath.Dir(path)}
	if err := parseFile(path, cfg); err != nil {
		return nil, err
	}

	if cfg.WorktreeBase == "" {
		cfg.WorktreeBase = "~/worktrees"
	}
	expanded, err := expandTilde(cfg.WorktreeBase)
	if err != nil {
		return nil, fmt.Errorf("expand worktree_base: %w", err)
	}
	cfg.WorktreeBase = expanded

	if cfg.BranchPrefix == "" {
		prefix, err := defaultBranchPrefix()
		if err != nil {
			return nil, err
		}
		cfg.BranchPrefix = prefix
	}

	return cfg, nil
}

func parseFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected key=value, got %q", path, lineNo, line)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "worktree_base":
			cfg.WorktreeBase = val
		case "branch_prefix":
			cfg.BranchPrefix = val
		default:
			return fmt.Errorf("%s:%d: unknown key %q", path, lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	return nil
}

func expandTilde(p string) (string, error) {
	if p == "" {
		return p, nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

func defaultBranchPrefix() (string, error) {
	if name, ok := gitUserName(); ok {
		return name, nil
	}
	if user, ok := whoami(); ok {
		return user, nil
	}
	return "", errors.New("could not determine branch prefix: git config user.name and whoami both failed")
}

func gitUserName() (string, bool) {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return "", false
	}
	return normalizePrefix(string(out)), normalizePrefix(string(out)) != ""
}

func whoami() (string, bool) {
	out, err := exec.Command("whoami").Output()
	if err != nil {
		return "", false
	}
	return normalizePrefix(string(out)), normalizePrefix(string(out)) != ""
}

func normalizePrefix(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
