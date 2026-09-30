{
  description = "resolved — scan code comments for stale GitHub issue/PR references";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs@{ flake-parts, ... }:
    let
      version = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
      vendorHash = "sha256-pLxzwnlyrR8dyUifi85jk4SlWeAqtyhdLMI1b+Iu3a0=";
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
              extraCheckInputs = [ pkgs.nilaway ];
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
              pkgs.nilaway
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
