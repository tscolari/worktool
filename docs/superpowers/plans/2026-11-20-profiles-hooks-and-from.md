# Profiles, hooks and `--from` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-task profiles of executable hooks (`post-start`, `pre-attach`, `session-command`, `list`, `pre-end`, `post-end`), plus `work start --from <ref>`.

**Architecture:** The new package `internal/hooks` resolves a profile directory and runs hook files with the `WORK_*` environment. `internal/gitx` gains start-point and branch-config helpers. Each command in `internal/cli` reads the workspace's profile from `branch.<branch>.workProfile` and calls its hooks at the points the spec fixes. Workspaces without a profile keep today's behaviour.

**Tech Stack:** Go 1.26, cobra, stdlib `testing` (the repo doesn't use testify), and the real `git` binary in tests.

**Spec:** `docs/superpowers/specs/2026-11-20-profiles-hooks-and-from-design.md`

## Global Constraints

- Profile directory: `<dir of resolved config path>/profiles/<name>/`. No new config key.
- Profile names: `^[a-z0-9][a-z0-9-]*$`.
- Profile record: `branch.<branch>.workProfile` in the main repo config.
- A missing hook file is a no-op. A hook file without an execute bit is an error, never skipped.
- Every hook receives `WORK_PROFILE`, `WORK_NAME`, `WORK_BRANCH`, `WORK_PATH` and `WORK_REPO`, except `list`, which receives `WORK_PROFILE` and `WORK_NAMES`.
- Hook working directory: `WORK_PATH` if it exists, else `WORK_REPO`. The spec says "the worktree", which can't hold for `post-end` or for `end`'s recovery path. This is a plan-level call.
- The `--from` check is `git rev-parse --verify --end-of-options <ref>^{commit}`. There is no fetch. The worktree is created with `git worktree add --no-track -b`.
- Errors follow the existing split: `userErr` (exit 1) for hook refusals and bad input, `sysErr` (exit 2) for unexpected failures.
- CI runs `go test -race ./...`, `go vet`, `gofmt` and golangci-lint `standard`. Tests must not need a real tmux.

## Review Focus

1. **Branch names with `/` used as git config subsections.** `branch.tester/TKT-1/desc.workProfile` must round-trip through write and read (Task 2).
2. **A hook file that lost its execute bit** after an edit must make the command fail with an error naming the file. It must not be skipped (Task 1).
3. **A profile directory deleted or renamed after `start`.** `attach` and `end` must fail with an error naming the profile. They must not carry on without hooks, which would be the spec's "silently stops applying" anti-goal (Tasks 4 and 5).
4. **A `--from` value that doesn't resolve or looks like an option**, e.g. `nope` or `-x`. The command must fail with a user error, and no branch or worktree may be created (Task 3).
5. **`session-command` output with a trailing newline or several lines.** Only the first line is used, with whitespace trimmed. Blank output means the default shell (Task 1, then end to end in Task 3).

---

### Task 1: Config directory and the `hooks` package

**Files:**
- Modify: `internal/config/config.go`. `Config` gains `Dir string`, which `Load` sets to `filepath.Dir(path)` of the resolved config path.
- Modify: `internal/config/config_test.go`
- Create: `internal/hooks/hooks.go`
- Test: `internal/hooks/hooks_test.go`

**Interfaces:**
- Produces:
  - `config.Config.Dir string`
  - `hooks.Event` (a string type) with the constants `PostStart = "post-start"`, `PreAttach = "pre-attach"`, `SessionCommand = "session-command"`, `List = "list"`, `PreEnd = "pre-end"` and `PostEnd = "post-end"`
  - `hooks.ValidName(name string) bool`
  - `hooks.Open(configDir, name string) (Profile, error)`. It returns a wrapped `hooks.ErrInvalidName` or `hooks.ErrNotFound`, so callers can use `errors.Is`.
  - `type Profile struct{ Name, Dir string }`
  - `type Env struct{ Name, Branch, Path, Repo string; Extra map[string]string }`. `Vars(profile string) []string` returns `WORK_PROFILE=` plus each non-empty field as `WORK_NAME`, `WORK_BRANCH`, `WORK_PATH` and `WORK_REPO`, plus the `Extra` entries.
  - `(Profile) Run(ev Event, env Env, stdin io.Reader, stdout, stderr io.Writer) error`. The hook's stdio is the given streams. A missing hook returns `nil`. A non-zero exit returns `*hooks.ExitError{Event, Profile, Err}`, whose `Error()` is `"<event> hook (profile <name>) failed: <err>"`.
  - `(Profile) Output(ev Event, env Env, stderr io.Writer) (string, error)`. stdout is captured and returned untrimmed, stdin is `os.Stdin`, and a missing hook returns `"", nil`.
  - `hooks.FirstLine(s string) string`: the first line, `strings.TrimSpace`d.

- [ ] **Step 1: Write the failing tests** in `internal/hooks/hooks_test.go`. Each one writes `#!/bin/sh` scripts into `t.TempDir()/profiles/<name>/` with mode `0o755`.
  - `TestOpen`, table-driven:
    - `"beam"` with an existing directory gives `Profile{Name:"beam", Dir:<cfg>/profiles/beam}`.
    - `"Beam"`, `"-x"`, `""` and `"a/b"` each give `errors.Is(err, ErrInvalidName)`.
    - `"ghost"` with no directory gives `errors.Is(err, ErrNotFound)`.
  - `TestRunMissingHookIsNoop`: `Run(PostStart, …)` on an empty profile directory returns `nil`.
  - `TestRunNotExecutable`: the hook file has mode `0o644`. The error is non-nil and contains the hook's path.
  - `TestRunEnvAndDir`: the hook runs `printf '%s|%s|%s|%s|%s|%s\n' "$WORK_PROFILE" "$WORK_NAME" "$WORK_BRANCH" "$WORK_PATH" "$WORK_REPO" "$WORK_BASE_REF"; pwd`. With `Env{Name:"T-1-x", Branch:"p/T-1/x", Path:<existing dir>, Repo:<repo dir>, Extra: {"WORK_BASE_REF":"refs/remotes/origin/v18"}}`, stdout is exactly `"beam|T-1-x|p/T-1/x|<path>|<repo>|refs/remotes/origin/v18\n<path>\n"`. Compare paths after `filepath.EvalSymlinks`, because macOS `/var` is a symlink to `/private/var`.
  - `TestRunDirFallsBackToRepo`: when `Env.Path` doesn't exist, `pwd` prints `Env.Repo`.
  - `TestRunNonZero`: the hook runs `exit 3`. The error satisfies `errors.As(err, &*ExitError)`, and `Error()` contains `"pre-end hook (profile beam) failed"`.
  - `TestFirstLine`, table-driven: `"tsh beams ssh x\n"` gives `"tsh beams ssh x"`. `"  a  \nb\n"` gives `"a"`. `"\n"` and `""` give `""`.
- [ ] **Step 2: Add a `config_test.go` case**: with `WORK_CONFIG=<tmp>/sub/config`, `cfg.Dir == <tmp>/sub`. Without `WORK_CONFIG`, `cfg.Dir == <home>/.config/work`.
- [ ] **Step 3: Run** `go test ./internal/hooks/ ./internal/config/`. Expected: FAIL, because `hooks` and `Config.Dir` don't exist yet.
- [ ] **Step 4: Implement** `internal/hooks/hooks.go` and `Config.Dir`. A hook path is `filepath.Join(p.Dir, string(ev))`. `os.Stat` returning `ErrNotExist` means a no-op. A mode with `&0o111 == 0` is the not-executable error. Use `exec.Command(path)`, with `Env = append(os.Environ(), env.Vars(p.Name)...)` and `Dir` from the Global Constraints rule.
- [ ] **Step 5: Run** `go test -race ./internal/hooks/ ./internal/config/`. Expected: PASS.
- [ ] **Step 6: Commit** `feat: hooks package and config dir`.

### Task 2: git helpers for start points and branch config

**Files:**
- Modify: `internal/gitx/gitx.go`
- Create: `internal/gitx/gitx_test.go`

**Interfaces:**
- Produces:
  - `gitx.WorktreeAdd(dir, path, branch, startPoint string) error`. This replaces the old three-argument form. It runs `git worktree add --no-track -b <branch> <path> <startPoint>`.
  - `gitx.ResolveBase(dir, ref string) (fullRef, commit string, err error)`:
    - `commit` comes from `rev-parse --verify --end-of-options <ref>^{commit}`.
    - `fullRef` comes from `rev-parse --symbolic-full-name <ref>`. It is `""` when the output is empty or `HEAD`, which covers a detached HEAD or a bare SHA.
    - A ref that doesn't resolve returns a wrapped `gitx.ErrUnknownRef`.
  - `gitx.BranchConfig(dir, branch, key string) (string, error)`: `git config --get branch.<branch>.<key>`. An exit code of 1 means `"", nil`.
  - `gitx.SetBranchConfig(dir, branch, key, value string) error`

- [ ] **Step 1: Write the failing tests.** The helper `newClone(t) string` creates a normal (non-bare) upstream repo with commits on `main` and `branch/v18`, clones it, and sets `user.name` and `user.email` locally. It returns the clone's directory.
  - `TestWorktreeAddNoTrack`: `WorktreeAdd(clone, wt, "tester/T-1/x", "origin/branch/v18")`. Afterwards, `BranchConfig(clone, "tester/T-1/x", "merge")` returns `""`, and the worktree's HEAD commit equals `origin/branch/v18`'s commit.
  - `TestResolveBase`, table-driven:
    - `"origin/branch/v18"` gives `fullRef == "refs/remotes/origin/branch/v18"`, and `commit` is that ref's SHA.
    - `"HEAD"` on `main` gives `"refs/heads/main"`.
    - `"HEAD"` after `git checkout --detach` gives `""`.
    - A full SHA gives `""` and the SHA itself.
    - `"nope"` gives `errors.Is(err, ErrUnknownRef)`.
    - `"-x"` gives `errors.Is(err, ErrUnknownRef)`.
  - `TestBranchConfigRoundTrip`: `SetBranchConfig(clone, "tester/T-1/x.y", "workProfile", "beam")`, then `BranchConfig` returns `"beam"`. After `git branch -D`, it returns `""` with a nil error.
- [ ] **Step 2: Run** `go test ./internal/gitx/`. Expected: FAIL, because the functions are undefined or `WorktreeAdd` has the wrong arity.
- [ ] **Step 3: Implement** the four functions. `ResolveBase` maps any `*exec.ExitError` from the `--verify` call to `ErrUnknownRef`. Update the single existing `WorktreeAdd` caller in `internal/cli/work.go` to pass `"HEAD"`, so the package still builds.
- [ ] **Step 4: Run** `go test -race ./internal/gitx/ && go build ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(gitx): start points, base resolution, branch config`.

### Task 3: `work start --profile/--from`, `post-start` rollback and `session-command`

**Files:**
- Modify: `internal/tmuxx/tmuxx.go`. `NewSessionDetached(name, cwd, command string)` and `NewSessionAttached(name, cwd, command string)` append `command` as the last tmux argument when it isn't empty.
- Modify: `internal/cli/work.go`, `cmd/work/main.go`, `internal/cli/attach.go` (callers of the new tmux signatures only; Task 4 adds the hooks there)
- Create: `internal/cli/profile.go`
- Create: `internal/cli/helpers_test.go`, `internal/cli/work_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1 and 2.
- Produces:
  - `cli.StartOptions{Profile, From string}`, with `RunWork(args []string, opts StartOptions, stdout, stderr io.Writer) error`.
  - In `profile.go`:
    - `workspaceProfile(cfg *config.Config, repoDir, branch string) (*hooks.Profile, error)`. It returns `nil, nil` when `workProfile` is unset. When the recorded profile's directory is gone, it returns a `userErr` naming the profile.
    - `sessionCommand(p *hooks.Profile, env hooks.Env, stderr io.Writer) (string, error)`. For a `nil` profile it returns `""`. Otherwise it returns `hooks.FirstLine` of `Output(SessionCommand, …)`.
  - Test helpers:
    - `setupConfig(t) (cfgDir, base string)` writes `worktree_base=<tmp>/wt` and `branch_prefix=tester`, and sets `WORK_CONFIG`.
    - `writeHook(t, cfgDir, profile string, ev hooks.Event, body string)`
    - `fakeTmux(t, hasSession bool) (logPath string)` puts a `tmux` script first on `PATH`, sets `TMUX=fake`, logs one line of arguments per call, and makes `has-session` exit 0 or 1.
    - `noTmux(t)` sets `PATH` to a temporary directory holding only a `git` symlink.
    - `newClone(t)`, copied from Task 2's helper. The packages differ, and the duplication is accepted.
    - Hooks record into `$HOOK_LOG`, an env var the test sets.

- [ ] **Step 1: Write the failing tests** in `work_test.go`. Each one runs `t.Chdir(clone)` and doesn't use `t.Parallel`, because of `t.Setenv`.
  - `TestStartNoProfileUnchanged`: with `noTmux` and no options, the branch `tester/T-1/x` and the worktree `<base>/T-1-x` exist, and `workProfile` is unset.
  - `TestStartFrom`: with `From:"origin/branch/v18"`, the worktree's HEAD equals `origin/branch/v18`, and the branch has no `merge` config.
  - `TestStartFromUnknownRef`, table-driven over `"nope"` and `"-x"`: the result is `*cli.Error` with `Code == 1`, `BranchExists` is false, and `<base>/T-1-x` doesn't exist.
  - `TestStartUnknownProfile`: `Profile:"ghost"` gives `Code == 1`, and nothing is created.
  - `TestStartPostStartEnv`: the `post-start` hook writes `$WORK_BASE_REF $WORK_BASE_COMMIT $WORK_BRANCH` to `$HOOK_LOG/post-start`. With `Profile:"beam", From:"origin/branch/v18"`, the file holds `refs/remotes/origin/branch/v18 <sha> tester/T-1/x`, and `workProfile == "beam"`.
  - `TestStartPostStartRollback`: the hook runs `exit 1`. The result has `Code == 1`, the branch and worktree are gone, and `workProfile` returns `""`.
  - `TestStartSessionCommand`: with `fakeTmux(t, false)`, `session-command` prints `printf 'tsh beams ssh b1\nignored\n'`. The tmux log contains `new-session -d -s T-1-x -c <path> tsh beams ssh b1`.
  - `TestStartSessionCommandBlank`: the hook prints `\n`. The tmux log line ends with `-c <path>`, with no extra argument.
- [ ] **Step 2: Run** `go test ./internal/cli/`. Expected: FAIL to compile, because `StartOptions` doesn't exist yet.
- [ ] **Step 3: Implement** the command in this order, and nothing is created before step 4:
  1. parse the argument
  2. `hooks.Open` if `Profile` is set, where `ErrInvalidName` and `ErrNotFound` become `userErr`
  3. `ResolveBase(cwd, From or "HEAD")`, where `ErrUnknownRef` becomes `userErr`
  4. the existing exists and path checks
  5. `WorktreeAdd(cwd, path, branch, From or "HEAD")`
  6. `SetBranchConfig(repo, branch, "workProfile", name)`
  7. `post-start` with `Extra{WORK_BASE_REF, WORK_BASE_COMMIT}`. On failure: `WorktreeRemove`, then `BranchDelete`, then return `userErr("%v; workspace rolled back", err)`.
  8. tmux, using `sessionCommand`
  
  In `main.go`, add the `--profile` and `--from` string flags to `startCmd`.
- [ ] **Step 4: Run** `go test -race ./internal/cli/ ./internal/tmuxx/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: work start --profile and --from`.

### Task 4: `pre-attach` and `session-command` on attach

**Files:**
- Modify: `internal/cli/attach.go`
- Create: `internal/cli/attach_test.go`

**Interfaces:**
- Consumes: `workspaceProfile` and `sessionCommand` (Task 3), and `gitx.BranchOfWorktree` and `gitx.CommonDir`.

- [ ] **Step 1: Write the failing tests.** Each first creates a workspace with `RunWork` under `noTmux`, then switches to `fakeTmux`.
  - `TestAttachPreAttachBothPaths`, table-driven over `hasSession` true and false: `pre-attach` appends a line to `$HOOK_LOG/pre-attach`, so the file has exactly one line afterwards.
    - With an existing session, the tmux log contains `switch-client -t T-1-x`, and `session-command` didn't run. That hook writes `$HOOK_LOG/session-command`, and the file must not exist.
    - Without a session, the tmux log contains `new-session -d -s T-1-x -c <path> <cmd>`.
  - `TestAttachPreAttachFails`: the hook runs `exit 1`. The result has `Code == 1`, and the tmux log contains no `has-session` call, so tmux was never touched.
  - `TestAttachProfileDirGone`: after `start`, run `os.RemoveAll(<cfg>/profiles/beam)`. The result has `Code == 1`, its message contains `beam`, and tmux was never called.
  - `TestAttachDetachedNoProfile`: a workspace without a profile whose worktree runs `git checkout --detach`. `attach` succeeds, as it does today.
- [ ] **Step 2: Run** `go test ./internal/cli/ -run Attach`. Expected: FAIL.
- [ ] **Step 3: Implement.** After the path stat, get the branch with `BranchOfWorktree(path)`. If that fails because of a detached HEAD, print `warning: <path> has a detached HEAD; profile hooks skipped` to stderr and carry on with no profile. Then run `workspaceProfile`, then `pre-attach`; any error from either returns before the `tmuxx.Installed` check. When a session is created, pass `sessionCommand` to `NewSession*`.
- [ ] **Step 4: Run** `go test -race ./internal/cli/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: pre-attach and session-command on attach`.

### Task 5: `pre-end` and `post-end`

**Files:**
- Modify: `internal/cli/workend.go`
- Create: `internal/cli/workend_test.go`

**Interfaces:**
- Consumes: `workspaceProfile` (Task 3).

- [ ] **Step 1: Write the failing tests.** Each creates a workspace with `--profile beam` under `noTmux` and runs `t.Chdir(<workspace>)`.
  - `TestEndPreEndBlocks`, table-driven over `force` false and true: `pre-end` writes `$WORK_FORCE` and then runs `exit 1`. Both cases return `Code == 1`, the worktree and branch still exist, and the log holds `""` and `"1"` respectively.
  - `TestEndDryRun`: `pre-end` writes `$WORK_DRY_RUN`. With `dryRun` true, the log holds `1`, nothing is deleted, `post-end` didn't run, and stdout contains `would run post-end hook (profile beam)`.
  - `TestEndPostEndAfterGitBeforeTmux`: under `fakeTmux(t, true)`, `post-end` writes `$WORK_PROFILE` to `$HOOK_LOG/post-end`, then `present` or `absent` depending on whether `$WORK_PATH` exists. The fake tmux script is extended to add `post-end-seen=yes` or `post-end-seen=no` to each log line, depending on whether `$HOOK_LOG/post-end` exists when it's called. Afterwards:
    - the branch is gone
    - the hook log holds `beam` and `absent`
    - the tmux log's last line is `kill-session -t T-1-x post-end-seen=yes`
  - `TestEndPostEndFailureWarns`: `post-end` runs `exit 1`. `end` returns `nil`, and stderr contains `warning:`.
  - `TestEndProfileDirGone`: after removing the profile directory, the result has `Code == 1`, and the worktree still exists.
  - `TestEndRecoveryUnknownBranch`: after `git worktree remove --force` and `git worktree prune` run directly, so the branch can't be resolved, `end` without `--force` returns `Code == 1` with a message naming the stale path. With `--force` it succeeds.
- [ ] **Step 2: Run** `go test ./internal/cli/ -run End`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - When `branch != ""`, call `workspaceProfile(cfg, repoDir, branch)` before the unmerged check and before anything is deleted.
  - Recovery path with `branch == ""` and `!force`: return `userErr`.
  - Run `pre-end` after worktool's own checks, with `Extra{WORK_FORCE, WORK_DRY_RUN}` set to `"1"` or `""`. Its error becomes `userErr`.
  - Under dry-run, also print `would run post-end hook (profile <name>)` when a profile is set.
  - Run `post-end` after branch deletion and before the tmux block. Its error goes to stderr as `warning: %v`.
- [ ] **Step 4: Run** `go test -race ./internal/cli/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: pre-end and post-end hooks`.

### Task 6: `list` annotations

**Files:**
- Modify: `internal/cli/list.go`
- Create: `internal/cli/list_test.go`

**Interfaces:**
- Consumes: `hooks.Open`, `Profile.Output`, `gitx.BranchConfig`, `gitx.CommonDir`.

- [ ] **Step 1: Write the failing tests** under `noTmux`, so the session column is dropped.
  - `TestListAnnotations`: three workspaces. `A` and `B` use profile `beam`, and `C` has none. The `beam` `list` hook appends `$WORK_NAMES` to `$HOOK_LOG/list`, then prints `A\t[beam: b1]` and `B\t[beam: b2]`. Each stdout line ends with its annotation. `C`'s line is unchanged from today's format. The hook ran once, and its `WORK_NAMES` held both `A` and `B`, one per line.
  - `TestListHookFails`: the hook runs `exit 1`. `list` returns `nil`, stderr contains `warning:`, and all three workspaces are printed without annotations.
  - `TestListUnknownName`: the hook also prints `ZZZ\tx`. The extra line is ignored.
- [ ] **Step 2: Run** `go test ./internal/cli/ -run List`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - First pass: collect `(name, branch, profile)` per workspace. The profile comes from `BranchConfig` on the worktree path; a lookup error means no profile.
  - Then call each profile's hook once with `Extra{WORK_NAMES: strings.Join(names, "\n")}` and `Env.Path` empty. Use `cfg.WorktreeBase` as the working directory, through `Env.Repo = cfg.WorktreeBase`.
  - Parse stdout into `name<TAB>annotation` lines.
  - A missing profile directory or a failed hook prints `warning:` and leaves the annotations empty.
  - Print the existing columns, then two spaces and the annotation when there is one.
- [ ] **Step 4: Run** `go test -race ./internal/cli/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: list annotations from profile hooks`.

### Task 7: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** Under `work start`, document `--from <ref>`: no fetch, `--no-track`, and that it replaces the `reset --hard` step.
- [ ] **Step 2:** Add a `## Profiles` section after `## Configuration`. It covers the directory layout, opting in with `--profile`, the profile being recorded on the branch, and the six-hook table from the spec. Include the environment table, plus `WORK_BASE_REF`, `WORK_BASE_COMMIT`, `WORK_FORCE`, `WORK_DRY_RUN` and `WORK_NAMES`. Add one paragraph on detach vs exit, explaining why both `pre-attach` and `session-command` exist, and the working-directory rule.
- [ ] **Step 3:** In `work end`, note that `pre-end` can block even with `--force`. Also note that `end` refuses the recovery path without `--force` when the branch can't be resolved.
- [ ] **Step 4: Run** `go build ./... && go test -race ./... && go vet ./... && test -z "$(gofmt -l .)"`. Expected: all pass, and no gofmt output.
- [ ] **Step 5: Commit** `docs: profiles, hooks and --from`.
