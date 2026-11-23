{
  description = "resolved — scan code comments for stale GitHub issue/PR references";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs@{ flake-parts, ... }:
    let
      version = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
      vendorHash = "sha256-YmEdjy7WQDNtrPPV7tduYAJwYdm/UtSt/HIDTSEZMYY=";
    in
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];

      perSystem = { pkgs, ... }:
        let
          # One static-analysis gate as a check derivation. It reuses packages'
          # Go module cache (same src + vendorHash) and runs `check` in the
          # checkPhase, so `nix flake check` runs the same gate locally and in CI.
          gate = { name, check, cgo ? false, extraCheckInputs ? [ ] }:
            pkgs.buildGoModule {
              pname = "resolved-gate-${name}";
              inherit version src vendorHash;
              nativeCheckInputs = [ pkgs.git ] ++ extraCheckInputs;
              env.CGO_ENABLED = if cgo then "1" else "0";
              checkPhase = ''
                runHook preCheck
                export HOME="$TMPDIR"
                export GOCACHE="$TMPDIR/go-cache"
                ${check}
                runHook postCheck
              '';
            };

          src = ./.;

          # nixpkgs' nilaway (2025-03-07) vendors an x/tools whose go/ssa panics
          # under Go 1.27. Track upstream main until nixpkgs ships a Go 1.27-ready
          # build, then drop this override. doCheck is off because upstream's own
          # test suite is OOM-killed on 7 GB CI runners.
          nilaway = pkgs.nilaway.overrideAttrs (_: {
            version = "0-unstable-2026-11-22";
            src = pkgs.fetchFromGitHub {
              owner = "uber-go";
              repo = "nilaway";
              rev = "7f797c9f2b67a10d81b207ea8c29241af77f85d7";
              hash = "sha256-H9Ex0dNdDTm5gHUeUMBVL8/02qW1IuHzTRLAgeh9/AU=";
            };
            doCheck = false;
            vendorHash = "sha256-qVmvDneq6V/q5UHZ/Cjjqd5/XPPNfvVGoxwg9nz4/Ds=";
          });
        in
        {
          packages.default = pkgs.buildGoModule {
            pname = "resolved";
            inherit version src vendorHash;
            nativeCheckInputs = [ pkgs.git ];
            env.CGO_ENABLED = "0";
            ldflags = [ "-s" "-w" "-X github.com/noamsto/resolved/internal/cli.version=${version}" ];
          };

          checks = {
            golangci-lint = gate {
              name = "golangci-lint";
              extraCheckInputs = [ pkgs.golangci-lint ];
              check = "golangci-lint run ./...";
            };
            nilaway = gate {
              name = "nilaway";
              extraCheckInputs = [ nilaway ];
              check = "nilaway -include-pkgs=github.com/noamsto/resolved ./...";
            };
            # The race detector needs cgo; test packages that spawn goroutines.
            race = gate {
              name = "race";
              cgo = true;
              check = "go test -race ./...";
            };
          };

          devShells.default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.gopls
              pkgs.golangci-lint
              nilaway
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
