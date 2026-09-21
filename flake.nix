{
  description = "NetBird Terraformer - imports NetBird resources into Terraform configuration";

  inputs.nixpkgs.url = "https://channels.nixos.org/nixos-unstable/nixexprs.tar.xz";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "netbird-terraformer";
          version = "0.1.0";
          src = ./.;

          # The tool uses only the Go standard library, so there is nothing to vendor.
          vendorHash = null;

          meta = {
            description = "Imports NetBird resources into Terraform configuration";
            mainProgram = "netbird-terraformer";
          };
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
            # OpenTofu rather than Terraform: it is not unfree.
            pkgs.opentofu
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixpkgs-fmt);
    };
}
