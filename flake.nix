{
  description = "gosherve";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs, ... }:
    let
      forAllSystems = nixpkgs.lib.genAttrs [
        "x86_64-linux"
        "aarch64-linux"
      ];

      pkgsForSystem =
        system:
        (import nixpkgs {
          inherit system;
          overlays = [ self.overlays.default ];
        });
    in
    {
      overlays.default =
        _final: prev:
        let
          inherit (prev) buildGoModule lib cacert;
          inherit (self) lastModifiedDate;
          commit = self.rev or self.dirtyRev or "dirty";
          version = "0.3.1-next";
        in
        {
          gosherve = buildGoModule {
            pname = "gosherve";
            inherit version;
            src = lib.cleanSource ./.;
            vendorHash = "sha256-/jC6zXGhSkZKDcSNn/SaSTuEYI6FWLwXQ94b72/RH7E=";
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

      devShells = forAllSystems (
        system:
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
              goreleaser
              zsh
            ];
            shellHook = "exec zsh";
          };
        }
      );
    };
}
