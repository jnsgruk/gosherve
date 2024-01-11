{
  description = "gosherve";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    formatters.url = "github:Gerschtli/nix-formatter-pack";
    formatters.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    { self
    , formatters
    , nixpkgs
    , ...
    }:
    let
      forAllSystems = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" ];

      pkgsForSystem = system: (import nixpkgs {
        inherit system;
        overlays = [ self.overlays.default ];
      });
    in
    {
      overlays.default = _final: prev:
        let
          inherit (prev) buildGoModule lib cacert;
          inherit (self) lastModifiedDate;
          commit = self.rev or self.dirtyRev or "dirty";
          version = "0.2.4-next";
        in
        {
          gosherve = buildGoModule {
            pname = "gosherve";
            inherit version;
            src = lib.cleanSource ./.;
            vendorHash = "sha256-pemMNEYjcpoHsQ0Cm6gjs+F4rWeT+GhYodsOWnD12fA=";
            buildInputs = [ cacert ];
            ldflags = [
              "-X main.version=${version}"
              "-X main.commit=${commit}"
              "-X main.date=${lastModifiedDate}"
            ];
          };
        };

      packages = forAllSystems (system: rec {
        inherit (pkgsForSystem system) gosherve;
        default = gosherve;
      });

      devShells = forAllSystems (system:
        let
          pkgs = pkgsForSystem system;
        in
        {
          default = pkgs.mkShell {
            name = "gosherve";
            NIX_CONFIG = "experimental-features = nix-command flakes";
            nativeBuildInputs = with pkgs; [
              go_1_21
              go-tools
              gofumpt
              gopls
              zsh
            ];
            shellHook = "exec zsh";
          };
        });

      formatter = forAllSystems (system:
        formatters.lib.mkFormatter {
          pkgs = pkgsForSystem system;
          config.tools = {
            deadnix.enable = true;
            nixpkgs-fmt.enable = true;
            statix.enable = true;
          };
        }
      );
    };
}

