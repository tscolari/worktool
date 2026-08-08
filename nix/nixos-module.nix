self:
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.work;
in
{
  options.programs.work = {
    enable = lib.mkEnableOption "work, a git worktree + tmux workspace manager";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "worktool.packages.\${system}.default";
      description = "The work package to install system-wide.";
    };
  };

  # work only reads $WORK_CONFIG or ~/.config/work/config, so there is no
  # system-wide config to render here. Per-user settings (branch_prefix in
  # particular) belong in the Home Manager module.
  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ cfg.package ];
  };
}
