{
  lib,
  buildGoModule,
  makeWrapper,
  installShellFiles,
  stdenv,
  git,
  tmux,
  version ? "0.1.0-dev",
}:

buildGoModule {
  pname = "work";
  inherit version;

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
    ];
  };

  vendorHash = "sha256-7K17JaXFsjf163g5PXCb5ng2gYdotnZ2IDKk8KFjNj0=";

  subPackages = [ "cmd/work" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  nativeBuildInputs = [
    makeWrapper
    installShellFiles
  ];

  # `work` probes `git config user.name` when no branch_prefix is configured,
  # which the config tests exercise.
  nativeCheckInputs = [ git ];

  # --suffix, not --prefix: the user's own tmux must win. A nixpkgs tmux client
  # attaching to an already-running system tmux server of a different version
  # fails with a protocol version mismatch, which would break `work attach`.
  # These are a fallback for machines that have neither tool.
  postInstall = ''
    wrapProgram $out/bin/work \
      --suffix PATH : ${
        lib.makeBinPath [
          git
          tmux
        ]
      }
  ''
  + lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd work \
      --bash <($out/bin/work completion bash) \
      --zsh <($out/bin/work completion zsh) \
      --fish <($out/bin/work completion fish)
  '';

  meta = {
    description = "Git worktree + tmux feature workspace manager";
    homepage = "https://github.com/tscolari/worktool";
    license = lib.licenses.mit;
    mainProgram = "work";
    platforms = lib.platforms.unix;
  };
}
