package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name             string
		fileContents     *string
		wantBase         func(home string) string
		wantPrefixSubstr string
		wantErr          string
	}{
		{
			name:             "missing file falls back to defaults",
			fileContents:     nil,
			wantBase:         func(home string) string { return filepath.Join(home, "worktrees") },
			wantPrefixSubstr: "",
		},
		{
			name:             "explicit values override defaults",
			fileContents:     ptr("worktree_base=/tmp/wt\nbranch_prefix=tscolari\n"),
			wantBase:         func(string) string { return "/tmp/wt" },
			wantPrefixSubstr: "tscolari",
		},
		{
			name:             "comments and blank lines are ignored",
			fileContents:     ptr("# comment\n\n  # indented\nbranch_prefix=foo\n"),
			wantBase:         func(home string) string { return filepath.Join(home, "worktrees") },
			wantPrefixSubstr: "foo",
		},
		{
			name:             "tilde in worktree_base expands to home",
			fileContents:     ptr("worktree_base=~/some/path\nbranch_prefix=p\n"),
			wantBase:         func(home string) string { return filepath.Join(home, "some", "path") },
			wantPrefixSubstr: "p",
		},
		{
			name:         "unknown key errors out",
			fileContents: ptr("nonsense=true\n"),
			wantErr:      "unknown key",
		},
		{
			name:         "malformed line errors out",
			fileContents: ptr("not_a_kv_line\n"),
			wantErr:      "expected key=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			cfgPath := filepath.Join(home, "config")
			if tt.fileContents != nil {
				if err := os.WriteFile(cfgPath, []byte(*tt.fileContents), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Setenv("WORK_CONFIG", cfgPath)
			} else {
				t.Setenv("WORK_CONFIG", filepath.Join(home, "does-not-exist"))
			}

			got, err := Load()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want := tt.wantBase(home); got.WorktreeBase != want {
				t.Errorf("WorktreeBase = %q, want %q", got.WorktreeBase, want)
			}

			if tt.wantPrefixSubstr != "" && !strings.Contains(got.BranchPrefix, tt.wantPrefixSubstr) {
				t.Errorf("BranchPrefix = %q, want substring %q", got.BranchPrefix, tt.wantPrefixSubstr)
			}
			if got.BranchPrefix == "" {
				t.Errorf("BranchPrefix is empty; default should never be empty")
			}
		})
	}
}

func TestWorkConfigEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "alt.config")
	if err := os.WriteFile(cfgPath, []byte("worktree_base=/explicit\nbranch_prefix=zz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("WORK_CONFIG", cfgPath)

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.WorktreeBase != "/explicit" {
		t.Errorf("WorktreeBase = %q, want /explicit", got.WorktreeBase)
	}
	if got.BranchPrefix != "zz" {
		t.Errorf("BranchPrefix = %q, want zz", got.BranchPrefix)
	}
}

func TestConfigDir(t *testing.T) {
	t.Run("WORK_CONFIG sets the directory", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("WORK_CONFIG", filepath.Join(tmp, "sub", "config"))

		got, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(tmp, "sub"); got.Dir != want {
			t.Errorf("Dir = %q, want %q", got.Dir, want)
		}
	})

	t.Run("default location", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("WORK_CONFIG", "")

		got, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".config", "work"); got.Dir != want {
			t.Errorf("Dir = %q, want %q", got.Dir, want)
		}
	})
}

func ptr(s string) *string { return &s }
