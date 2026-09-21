{
  description = "NetBird Terraformer - imports NetBird resources into Terraform configuration";

  inputs.nixpkgs.url = "https://channels.nixos.org/nixos-unstable/nixexprs.tar.xz";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        default = netbird-terraformer;

        netbird-terraformer = pkgs.buildGoModule {
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

        # The state helpers, so consumers can run them without vendoring the
        # repo. They shell out to `terraform`, which stays the caller's to
        # provide: pinning one here would override the version the project uses.
        helpers = pkgs.stdenv.mkDerivation {
          pname = "netbird-terraformer-helpers";
          version = "0.1.0";
          src = ./.;
          dontBuild = true;
          installPhase = ''
            runHook preInstall
            install -Dm755 prune_imports.sh   $out/bin/nbt-prune-imports
            install -Dm755 reconcile_state.sh $out/bin/nbt-reconcile-state
            install -Dm644 backend-env.sh     $out/share/netbird-terraformer/backend-env.sh
            runHook postInstall
          '';
          meta.description = "State reconciliation helpers for netbird-terraformer";
        };
      });

      apps = forAllSystems (pkgs:
        let p = self.packages.${pkgs.stdenv.hostPlatform.system}; in
        {
          default = {
            type = "app";
            program = "${p.netbird-terraformer}/bin/netbird-terraformer";
            meta.description = "Import NetBird resources into Terraform configuration";
          };
          prune-imports = {
            type = "app";
            program = "${p.helpers}/bin/nbt-prune-imports";
            meta.description = "Drop import blocks for resources already in state";
          };
          reconcile-state = {
            type = "app";
            program = "${p.helpers}/bin/nbt-reconcile-state";
            meta.description = "Reconcile state with configuration without touching NetBird";
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
