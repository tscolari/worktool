{
  description = "work — git worktree + tmux feature workspaces";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      version = "0.0.6" + lib.optionalString (self ? shortRev) "-${self.shortRev}";

      workFor = pkgs: pkgs.callPackage ./nix/package.nix { inherit version; };
    in
    {
      overlays.default = final: _prev: {
        work = workFor final;
      };

      packages = forAllSystems (
        pkgs:
        let
          work = workFor pkgs;
        in
        {
          inherit work;
          default = work;
        }
      );

      apps = forAllSystems (
        pkgs:
        let
          work = workFor pkgs;
          app = {
            type = "app";
            program = lib.getExe work;
            meta = { inherit (work.meta) description; };
          };
        in
        {
          work = app;
          default = app;
        }
      );

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
            pkgs.git
            pkgs.tmux
          ];
        };
      });

      # Builds the package, which runs `go test ./...` via buildGoModule's checkPhase.
      checks = forAllSystems (pkgs: {
        build = workFor pkgs;
      });

      homeManagerModules.default = import ./nix/hm-module.nix self;
      nixosModules.default = import ./nix/nixos-module.nix self;

      formatter = forAllSystems (pkgs: pkgs.nixfmt-tree);
    };
}
