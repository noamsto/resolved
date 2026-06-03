{
  description = "resolved CLI dev environment";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = import nixpkgs { inherit system; };
      in {
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
        packages.default = pkgs.buildGoModule {
          pname = "resolved";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # set after first `nix build` reports the hash
          env.CGO_ENABLED = "1";
        };
      });
}
