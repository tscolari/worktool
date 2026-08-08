self:
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.work;

  settings = lib.filterAttrs (_: v: v != null) (
    {
      worktree_base = cfg.worktreeBase;
      branch_prefix = cfg.branchPrefix;
    }
    // cfg.extraSettings
  );

  configText = lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: "${k}=${v}") settings);
in
{
  options.programs.work = {
    enable = lib.mkEnableOption "work, a git worktree + tmux workspace manager";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "worktool.packages.\${system}.default";
      description = "The work package to install.";
    };

    worktreeBase = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "~/inflight";
      description = ''
        Directory where git worktrees are created (`worktree_base`).
        Null keeps work's built-in default of `~/worktrees`.
      '';
    };

    branchPrefix = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "tscolari";
      description = ''
        Prefix added to every branch name (`branch_prefix`), producing
        `<prefix>/<ticket>/<description>`. Null falls back to
        `git config user.name`, then `whoami`.
      '';
    };

    extraSettings = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = {
        some_future_key = "value";
      };
      description = ''
        Escape hatch for config keys not yet exposed as options.
        Note that work rejects unknown keys with a hard error.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    # Lands at ~/.config/work/config, where config.Load() looks. Omitted
    # entirely when nothing is set, so work's own defaults apply.
    xdg.configFile."work/config" = lib.mkIf (settings != { }) {
      text = configText + "\n";
    };
  };
}
