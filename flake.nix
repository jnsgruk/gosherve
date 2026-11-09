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
          inherit (prev) lib cacert;
          buildGoModule = prev.buildGoModule.override { go = prev.go_1_27; };
          inherit (self) lastModifiedDate;
          commit = self.rev or self.dirtyRev or "dirty";
          version = "0.5.0";
        in
        {
          gosherve = buildGoModule {
            pname = "gosherve";
            inherit version;
            src = lib.cleanSource ./.;
            vendorHash = "sha256-s21B5zCGNEYnhhIOYXH4cNaM3/U9vMLuhe3MOrwp5+M=";
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
    };
}
