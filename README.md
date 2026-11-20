# WORK

[![CI](https://github.com/tscolari/worktool/actions/workflows/ci.yml/badge.svg)](https://github.com/tscolari/worktool/actions/workflows/ci.yml)

{Worktree, Tmux session} Manager

## The flow

They assist me with how I like to work in a codebase:

1. From inside the repository I want to work on
2. Create a new branch, prefixed with my handler, the ticket code and followed by a small description
3. Create a worktree for that branch and jump into it
4. Create a new tmux session, named after the ticket code and description
5. Jump into the session and start working

With a quick tear down
1. Delete the branch / worktree and tmux session

---

## With the tool

1. `cd ./codebase`
2. `work start TKT-1234/creating-api-mocks`
3. ... work
4. `work end` (from the worktree dir)

And to get back into it later, from anywhere:

```
work list                                # what exists, and what has a live session
work attach TKT-1234-creating-api-mocks  # jump in (creating the session if it's gone)
```

## Names

One argument drives everything. `work start TKT-1234/creating-api-mocks` gives you:

| | |
|---|---|
| branch | `<branch_prefix>/TKT-1234/creating-api-mocks` |
| worktree dir | `<worktree_base>/TKT-1234-creating-api-mocks` |
| tmux session | `TKT-1234-creating-api-mocks` |

The **workspace name** is the argument with the slash swapped for a dash:
`TKT-1234-creating-api-mocks`. That single name is what `list`, `attach` and `end` work
with — only `start` takes the slashed form.

## Commands

### `work start TICKET/kebab-description`

Creates the branch (from the current HEAD), the worktree, and the tmux session, then drops
you into it. Run it from inside the repository you want to branch from.

It refuses if the branch already exists, or if the worktree path is already taken — in both
cases it tells you the path to `cd` into instead.

If you're already inside tmux it creates the session detached and switches your client to
it. If you're not, it replaces itself with `tmux new-session`. Either way you end up in the
new workspace. If tmux isn't installed at all, you still get the branch and worktree, with
a warning that the session was skipped.

Flags:
- `--from <ref>` — start the branch at `<ref>` instead of HEAD, e.g.
  `work start TKT-1234/backport --from origin/branch/v18`. It saves you a `reset --hard`
  once you're inside the worktree. Nothing is fetched, so the ref is used exactly as it is
  locally; `git fetch` first if it might be stale. The new branch gets no upstream, even
  when `<ref>` is a remote branch, so a plain `git push` can't land on the base by
  accident.
- `--profile <name>` — run a [profile](#profiles)'s hooks for this workspace. The choice
  is recorded on the branch, and `attach`, `list` and `end` pick it up from there without
  the flag.

### `work attach <workspace-name>`

The everyday way back into a workspace. It takes the workspace name (the dashed form) and
works **from anywhere** — no need to `cd` into the repo or the worktree first.

If a session for that workspace is running, you get connected to it. If there isn't one, it
creates one rooted at the worktree and says so. That's the whole trick: reconnecting to
live work and restoring work whose session is gone are the same command.

```
work attach TKT-1234-creating-api-mocks
```

Which makes recovering after a reboot a non-event — tmux sessions don't survive one, but
worktrees and branches do, so `work list` still shows everything and `attach` puts the
sessions back one at a time as you need them.

Details:

- Inside tmux it switches your client; outside tmux it attaches. Both do the right thing.
- It never creates worktrees. The workspace directory has to already exist under
  `worktree_base`, otherwise it errors out.
- Without tmux installed, it falls back to printing the `cd` path.
- Workspace names tab-complete (see [completion](#work-completion-bashzshfish)), so the
  dashed name is cheap to type.

### `work list`

Every workspace under `worktree_base`, with its branch and whether a tmux session exists:

```
TKT-1234-creating-api-mocks   tscolari/TKT-1234/creating-api-mocks   [session]
TKT-5678-flaky-consumer-test  tscolari/TKT-5678/flaky-consumer-test  [no session]
```

Prints `no active workspaces` when there's nothing there. If tmux isn't installed the
session column is dropped entirely.

### `work end`

Tears the current workspace down: worktree, branch, and tmux session. Run it from inside
the workspace directory — it refuses to run anywhere else.

It reads the branch from the worktree's HEAD rather than re-deriving it from the directory
name, so it stays correct even if things drift. Before deleting anything it checks that:

- the branch isn't checked out in another worktree (if it is, remove that one first);
- the branch has no commits missing from its upstream — pass `--force` to delete anyway.
  A branch with no upstream configured, which is the normal case for fresh work, passes
  this check.

If the workspace has a [profile](#profiles), its `pre-end` hook runs after these checks
and can refuse the teardown. `--force` doesn't skip it; the hook sees `WORK_FORCE=1` and
decides for itself.

When the worktree directory is already gone and its branch can no longer be found, `end`
can't tell whether the workspace had a profile. If you have any profiles, it refuses unless
you pass `--force`; without profiles it cleans up as it always did.

Flags:
- `--force` — skip the unmerged-commits check (and allow the cleanup above)
- `--dry-run` — print what would happen without doing anything

### `work cleanup-branches`

Deletes local branches already merged into the repository's default branch. The default
branch is read from `refs/remotes/origin/HEAD`, falling back to `main` then `master`.

It lists the candidates and asks before deleting:

```
work cleanup-branches
work cleanup-branches --yes        # or -y, to skip the prompt
```

This touches branches only — it doesn't remove worktrees or kill sessions. Use `work end`
for that.

### `work completion bash|zsh|fish`

Prints the shell completion script, which is what makes workspace names tab-complete for
`work attach`. Add `eval "$(work completion zsh)"` (or `bash`, `fish`) to your shell rc.
Not needed if you installed via Nix — those completions are installed as real files.

### Also

`work --help` (and `work <command> --help`) for usage, `work --version` for the version.

## Requirements

`git` is required. `tmux` is optional: without it you still get worktree and branch
management, and `list` still works — you just lose the session handling and the session
column.

---

## Configuration

`~/.config/work/config`, or any path you point `WORK_CONFIG` at.

```
worktree_base=~/inflight
branch_prefix=myname
```

Both keys are optional:

- **`worktree_base`** — where worktrees get created. Defaults to `~/worktrees`. A leading
  `~` is expanded.
- **`branch_prefix`** — the first segment of every branch name. Defaults to
  `git config user.name`, lowercased with spaces turned into hyphens, falling back to
  `whoami`.

The format is one `key=value` per line. Blank lines and `#` comments are ignored, and an
unknown key is an error rather than something silently skipped. See
[`config.example`](./config.example).

---

## Profiles

A profile lets a workspace do more than branch + worktree + tmux, without teaching
worktool what that "more" is. It's a directory of executables, one per event:

```
~/.config/work/profiles/<name>/
  post-start
  pre-attach
  session-command
  list
  pre-end
  post-end
```

The directory sits next to the config file, so `WORK_CONFIG` moves it too. Names match
`^[a-z0-9][a-z0-9-]*$`. Every hook is optional, but a hook file that isn't executable is an
error, not something quietly skipped.

You opt in per workspace with `work start --profile <name>`. worktool records the name in
the main repo's config as `branch.<branch>.workProfile`; workspaces without it run no hooks
and behave exactly as before.

| hook | runs | contract |
|---|---|---|
| `post-start` | in `start`, after the worktree exists, before tmux | A non-zero exit rolls the worktree and branch back. Cleaning up what the hook itself created is up to the hook. |
| `pre-attach` | on every `attach`, before tmux is touched | A non-zero exit aborts `attach`. |
| `session-command` | when `start` or `attach` creates a tmux session | The first line of stdout becomes the session's command; empty means your default shell. Not run when `attach` finds a live session. |
| `list` | once per profile in `work list` | Gets every workspace of the profile in `WORK_NAMES`, prints `name<TAB>annotation` lines, appended as a last column. A failure is a warning. |
| `pre-end` | in `end`, after worktool's checks, before anything is deleted | A non-zero exit aborts `end`, even with `--force`. Under `--dry-run` it runs and must not change anything. |
| `post-end` | in `end`, after the worktree and branch are gone, before the tmux session is killed | For teardown. A failure is a warning. |

Leaving a workspace can mean a detach (the session stays and the next `attach` just
switches to it) or an exit (the session ends and the next `attach` creates a new one).
`session-command` only sees the second, so anything that has to happen on every return
belongs in `pre-attach`.

Hooks inherit your environment, your terminal (except `session-command` and `list`,
whose stdout is read), and these variables:

| variable | value |
|---|---|
| `WORK_PROFILE` | the profile name |
| `WORK_NAME` | workspace name, e.g. `TKT-1234-creating-api-mocks` |
| `WORK_BRANCH` | the full branch name |
| `WORK_PATH` | the worktree path |
| `WORK_REPO` | the main repository directory |
| `WORK_BASE_REF`, `WORK_BASE_COMMIT` | `post-start` only: the base's full ref name (empty for a bare SHA or a detached HEAD) and its commit |
| `WORK_FORCE`, `WORK_DRY_RUN` | `pre-end` only: `1` or empty |
| `WORK_NAMES` | `list` only, instead of the workspace variables: one workspace name per line |

A hook runs inside the worktree, or in the main repository once the worktree is gone
(`post-end`, and `end`'s recovery path).

---

## Install

### Nix

```
nix run github:tscolari/worktool -- start TKT-1234/creating-api-mocks   # try it
nix profile install github:tscolari/worktool                            # keep it
```

As a flake input, with the Home Manager module:

```nix
{
  inputs.worktool.url = "github:tscolari/worktool";

  # ... then in your Home Manager config:
  imports = [ inputs.worktool.homeManagerModules.default ];

  programs.work = {
    enable = true;
    worktreeBase = "~/inflight";
    branchPrefix = "tscolari";
  };
}
```

That installs the binary and writes `~/.config/work/config` for you. There is
also `nixosModules.default` (`programs.work.enable`) for a system-wide install,
and an `overlays.default` exposing `pkgs.work`.

Shell completions are installed as real files, so Nix users don't need the
`eval` line above.

### Prebuilt binary

Grab a tarball for your platform from the
[releases page](https://github.com/tscolari/worktool/releases) — linux and
darwin, amd64 and arm64. Each archive contains the `work` binary,
`config.example`, and shell completions under `completions/`.

```
tar xzf work_*_linux_amd64.tar.gz
install -m755 work ~/.local/bin/work
```

### Go

```
go install github.com/tscolari/worktool/cmd/work@latest
```
