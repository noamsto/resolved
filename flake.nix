{
  description = "resolved — scan code comments for stale GitHub issue/PR references";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs@{ flake-parts, ... }:
    let
      version = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
    in
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

      perSystem = { pkgs, ... }: {
        packages.default = pkgs.buildGoModule {
          pname = "resolved";
          inherit version;
          src = ./.;
          vendorHash = "sha256-BIf+Yy3S7+Ie1/q9D/iDegz83/dLpYciwt03T/NlAbY=";
          nativeCheckInputs = [ pkgs.git ];
          env.CGO_ENABLED = "0";
          ldflags = [ "-s" "-w" "-X github.com/noamsto/resolved/internal/cli.version=${version}" ];
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.goreleaser
            pkgs.gh
            pkgs.git
          ];
        };
      };

      # `programs.resolved.enable = true;` puts the resolved CLI on PATH so the
      # /resolved:stale plugin finds it — no auto-download.
      flake.homeManagerModules.default = { config, lib, pkgs, ... }: {
        options.programs.resolved.enable =
          lib.mkEnableOption "the resolved CLI on PATH for the resolved Claude Code plugin";
        config = lib.mkIf config.programs.resolved.enable {
          home.packages = [ inputs.self.packages.${pkgs.stdenv.hostPlatform.system}.default ];
        };
      };
    };
}
