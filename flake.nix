{
  description = "meshy - a Go CLI for meshy.AI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = nixpkgs.lib.genAttrs systems;

      pkgsFor =
        system:
        import nixpkgs {
          inherit system;
          overlays = [ self.overlays.default ];
        };
    in
    {
      # Use as: nixpkgs.overlays = [ meshy-cli.overlays.default ];
      overlays.default = final: _prev: {
        meshy = final.callPackage ./package.nix { };
      };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          meshy = pkgs.meshy;
          default = pkgs.meshy;
        }
      );

      apps = forAllSystems (
        system:
        let
          meshy = self.packages.${system}.meshy;
          app = {
            type = "app";
            program = nixpkgs.lib.getExe meshy;
          };
        in
        {
          meshy = app;
          default = app;
        }
      );

      # `nix flake check` builds the package.
      checks = forAllSystems (system: {
        meshy = self.packages.${system}.meshy;
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              # Built package available as `meshy` in the shell.
              meshy
              cacert
              curl
              delve
              git
              go
              go-tools
              golangci-lint
              gopls
              gotools
              jq
              just
              nixfmt
            ];

            env = {
              CGO_ENABLED = "0";
              GOFLAGS = "-mod=mod";
            };

            shellHook = ''
              echo "meshy-cli dev shell"
              echo "Go: $(go version)"
              echo "meshy: $(command -v meshy) ($(meshy version 2>/dev/null || echo built package))"
              echo "Set MESHY_API_KEY to talk to meshy.AI."
            '';
          };
        }
      );

      formatter = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        pkgs.writeShellApplication {
          name = "meshy-format";
          runtimeInputs = [ pkgs.nixfmt ];
          text = ''
            if [ "$#" -eq 0 ]; then
              exec nixfmt flake.nix package.nix
            fi

            exec nixfmt "$@"
          '';
        }
      );
    };
}
