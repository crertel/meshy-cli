{
  description = "meshy - a Go CLI for meshy.AI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems =
        f:
        nixpkgs.lib.genAttrs systems (
          system:
          f (
            import nixpkgs {
              inherit system;
            }
          )
        );
    in
    {
      packages = forAllSystems (pkgs: {
        meshy = pkgs.buildGoModule {
          pname = "meshy";
          version = "0.1.0";
          src = pkgs.lib.cleanSource ./.;
          vendorHash = "sha256-tFFy7I2Tj0kXqh+gs694Bc02K2JCzQhigeYEhUi+K5E=";
          subPackages = [ "cmd/meshy" ];
        };

        default = self.packages.${pkgs.stdenv.hostPlatform.system}.meshy;
      });

      apps = forAllSystems (pkgs: {
        meshy = {
          type = "app";
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.meshy}/bin/meshy";
        };

        default = {
          type = "app";
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.default}/bin/meshy";
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
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
            echo "Set MESHY_API_KEY to talk to meshy.AI."
          '';
        };
      });

      formatter = forAllSystems (
        pkgs:
        pkgs.writeShellApplication {
          name = "meshy-format";
          runtimeInputs = [ pkgs.nixfmt ];
          text = ''
            if [ "$#" -eq 0 ]; then
              exec nixfmt flake.nix
            fi

            exec nixfmt "$@"
          '';
        }
      );
    };
}
