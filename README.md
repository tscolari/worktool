# WORK

A vibecoded helper I created for my day to day usage.

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
`eval` line below.

### Go

```
go install github.com/tscolari/work/cmd/work@latest
```

---

## With the tool

1. `cd ./codebase`
2. `work start TKT-1234/creating-api-mocks`
3. ... work
4. `work end` (from the worktree dir)

Flags for `work end`:
- `--force` — skip the unmerged-commits check
- `--dry-run` — print what would happen without doing anything

### After a reboot

tmux sessions don't survive a reboot, but the worktrees and branches do.

```
work list                          # see all workspaces and whether a session exists
work attach TKT-1234-creating-api-mocks  # reconnect (creates a new session if needed)
```

Tab completion for workspace names: add `eval "$(work completion zsh)"` (or `bash`) to your shell rc. Not needed if you installed via Nix.

### Cleaning up

```
work cleanup-branches              # delete local branches already merged into main
work cleanup-branches --yes        # skip the confirmation prompt
```

--

## Configuration

~/.config/work/config (or set `WORK_CONFIG`)
```
worktree_base=~/inflight
branch_prefix=myname
```
