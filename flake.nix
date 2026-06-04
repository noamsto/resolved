{
  description = "resolved — scan code comments for stale GitHub issue/PR references";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

      perSystem = { pkgs, ... }: {
        packages.default = pkgs.buildGoModule {
          pname = "resolved";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-UFtvqOD3n/0zoFqOv20o79RqTnDSfFxlCkQKkcW1dxk=";
          env.CGO_ENABLED = "1"; # tree-sitter needs cgo
          ldflags = [ "-s" "-w" ];
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gcc # CGO for tree-sitter
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.goreleaser
            pkgs.gh
            pkgs.git
          ];
        };
      };
    };
}
