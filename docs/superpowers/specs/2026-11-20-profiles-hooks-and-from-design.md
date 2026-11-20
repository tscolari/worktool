# Profiles, hooks and `--from`

## Why

Some work needs more than a branch, a worktree and a tmux session. The first case is Teleport work that runs inside a beam. The code moves there, the session runs on the beam, and the laptop keeps a read-only copy. worktool should make that possible without learning anything about beams. Personal projects keep the current behaviour, with nothing new to configure.

A second, smaller need: starting work from a base other than the current HEAD. Today the flow is `work start` followed by `git reset --hard <base>` inside the worktree. That works locally, but nothing records the base, and once the work lives in a beam the reset happens on the wrong machine.

## Goals

- Opt in per task with `work start TICKET/desc --profile <name>`. A profile is a directory of executable hooks.
- The workspace remembers its profile. `attach`, `list` and `end` run the same profile's hooks later, with no flag.
- `work start --from <ref>` creates the branch at `<ref>` and passes the base to the hooks.
- Without `--profile`, every command behaves exactly as it does today.

## Non-goals

- Anything beam-specific. The beam integration is a separate binary that implements a profile.
- Per-repo hooks, or hooks that run for every workspace.
- Changing the profile of an existing workspace.
- Fetching before `--from`. The ref is used as it is locally.

## Anti-goals

- A hook failure that leaves a half-created workspace, or a workspace deleted after its hook refused.
- A profile that silently stops applying. Example: the profile is lost before `end` runs, and `end` tears down without asking its hook.

## Approaches considered

1. **A wrapper binary around `work`.** worktool stays untouched. Rejected because `work attach`, `work list` and `work end` all bypass the wrapper, and `end` could delete a workspace whose beam still holds work that hasn't synced.
2. **Per-repo hooks in git config.** No new flags. Rejected because some work in the same repo should stay local, like quick fixes and reviews.
3. **Per-task profiles with hook files (picked).** The opt-in happens at `start` and is recorded on the branch. Workspaces without a profile run no hooks.

The hooks are executable files in a profile directory, not commands in the config file. The config parser rejects unknown keys, and a hook file needs no new key. (My call.)

## Design

### Profiles

- A profile lives at `<config dir>/profiles/<name>/`, where `<config dir>` is the directory of the resolved config path. That is `~/.config/work/profiles/` by default, or next to `WORK_CONFIG`. No new config key.
- Names match `^[a-z0-9][a-z0-9-]*$`. `--profile` with a missing directory is a user error.
- `start` stores the profile in the main repo's config as `branch.<branch>.workProfile`. Later commands read it from there.
- A hook is a file named after its event. A missing hook is a no-op. A hook file that exists but isn't executable is an error. It is never skipped.

### Hook environment

Every hook runs with its working directory set to the worktree and receives:

| variable | value |
|---|---|
| `WORK_PROFILE` | profile name |
| `WORK_NAME` | workspace name, e.g. `TKT-1-desc` |
| `WORK_BRANCH` | full branch name |
| `WORK_PATH` | worktree path |
| `WORK_REPO` | main repo directory (`gitx.CommonDir`) |

There are event-specific extras, listed below. Hook stdin, stdout and stderr are connected to the terminal, except where a hook's stdout is data (`session-command`, `list`).

### Events

| hook | when | contract |
|---|---|---|
| `post-start` | after the worktree and the `workProfile` config exist, before tmux | Extras: `WORK_BASE_REF` and `WORK_BASE_COMMIT`. On a non-zero exit, worktool removes the worktree and the branch, which also drops the branch config, then exits non-zero. Cleaning up anything the hook itself created is the hook's job. |
| `pre-attach` | on every `attach`, before checking for the tmux session | A non-zero exit aborts `attach` and shows the hook's stderr. The hook decides what is fatal. For example, an unreachable origin can be a warning followed by exit 0. |
| `session-command` | when `start` or `attach` creates a tmux session | The first line of stdout is the shell command for `tmux new-session`. Empty output means the default shell. A non-zero exit fails the command, and no session is created. It doesn't run when `attach` finds an existing session. |
| `list` | once per profile in `work list`, not once per workspace | Extra: `WORK_NAMES`, a newline-separated list of the workspaces with this profile. stdout has `name<TAB>annotation` lines, appended as a last column. A failure prints a warning and leaves the column empty. `list` itself never fails. |
| `pre-end` | in `end`, after worktool's own checks and before anything is deleted | Extras: `WORK_FORCE` and `WORK_DRY_RUN`, set to `1` or empty. A non-zero exit aborts `end` and shows the hook's stderr. `--force` doesn't skip the hook; it only passes `WORK_FORCE=1`. Under `--dry-run` the hook runs and must not change anything. |
| `post-end` | in `end`, after the worktree and branch are removed, before the tmux session is killed | Teardown, such as removing the beam. A failure prints a warning. The workspace is already gone. |

The command line for the tmux pane comes from the hook, unmodified. When it exits, tmux closes the pane. A hook that wants a local shell afterwards adds it itself.

`pre-attach` and `post-end` are in addition to the four hooks we discussed (my calls).

`pre-attach` exists because both detach and exit are everyday ways of leaving a workspace, and `session-command` only sees the second one:

- **Detach** (`prefix d`). The session stays alive. The next `attach` switches to it, and `session-command` doesn't run.
- **Exit.** The session ends. The next `attach` creates a new one, and `session-command` runs.

Anything that has to happen on every return goes in `pre-attach`, and it then works the same for both. For the beam profile, that means checking that sync is running and refreshing upstream.

`post-end` exists because, with only `pre-end`, teardown would happen before worktool's deletion. A failed `git worktree remove` would then leave a workspace whose beam is already gone. `post-end` runs before the session is killed, because `end` kills tmux last for a reason: it may SIGHUP its own process.

### `end` and the profile

- `end` reads `branch.<branch>.workProfile` before `git branch -D`, which deletes the branch's config section.
- In the recovery path the worktree directory is already gone. If the branch can still be resolved through worktree metadata, the profile is read as usual. If it can't, `end` fails with a user error naming the stale path, unless `--force` is passed. This protects against the anti-goal of a profile that silently stops applying.

### `--from <ref>`

- `git rev-parse --verify --end-of-options <ref>^{commit}` must succeed. Otherwise it's a user error. No fetch.
- The worktree is created with `git worktree add --no-track -b <branch> <path> <ref>`. `--no-track` stops git's default `branch.autoSetupMerge` from making the base the upstream. That would change what `work end`'s unmerged check compares against, and could make a plain `git push` target the base.
- The hooks get `WORK_BASE_REF`, the full ref name from `git rev-parse --symbolic-full-name` (for example `refs/remotes/origin/branch/v18`), and `WORK_BASE_COMMIT`.
- Without `--from`, the base is HEAD: `WORK_BASE_REF` is HEAD's symbolic full name, empty when detached, and `WORK_BASE_COMMIT` is HEAD's commit.
- `--from` works with or without `--profile`.

## Left to the builder

- How `Config` exposes the config directory, and where the hook runner lives. A small `internal/hooks` package is the obvious place.
- Error wording, following the existing `userErr` and `sysErr` split.
- Tests:
  - the `post-start` rollback
  - `pre-end` blocking, with and without `--force` and `--dry-run`
  - `pre-attach` running both when the session exists and when `attach` creates it, and its non-zero exit aborting before tmux is touched
  - the profile surviving until `end` reads it
  - `--no-track`, by checking that the new branch has no `branch.<b>.merge`
  - `list` with several profiles
- Update the README: the profiles section, `--from`, and the hook environment table.
