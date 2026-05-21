{
  description = "Local proxy for old CUSP device web interfaces";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      eachSystem = f:
        nixpkgs.lib.genAttrs systems (system:
          f (import nixpkgs { inherit system; }));
    in
    {
      packages = eachSystem (pkgs: {
        default = pkgs.buildGoModule {
          pname = "cusp-local-proxy";
          version = "0.1.0";
          src = ./.;
          vendorHash = null;
          subPackages = [ "cmd/cusp-local-proxy" ];
        };
      });

      apps = eachSystem (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.default}/bin/cusp-local-proxy";
        };

        build-windows = {
          type = "app";
          program = "${pkgs.writeShellApplication {
            name = "build-windows";
            runtimeInputs = [ pkgs.go ];
            text = ''
              mkdir -p dist
              GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o dist/cusp-local-proxy.exe ./cmd/cusp-local-proxy
              echo "wrote dist/cusp-local-proxy.exe"
            '';
          }}/bin/build-windows";
        };
      });

      devShells = eachSystem (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
          ];
        };
      });
    };
}
