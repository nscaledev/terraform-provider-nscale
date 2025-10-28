{
  description = "Terraform Provider Nscale";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        go = pkgs.go_1_25;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            terraform
            goreleaser
            git
            gnumake
          ];

          shellHook = ''
            export GOPATH=$HOME/go
            export PATH=$GOPATH/bin:$PATH
          '';
        };

        packages.default = pkgs.buildGoModule {
          pname = "terraform-provider-nscale";
          version = "dev";
          
          src = ./.;
          
          vendorHash = null;
          
          buildInputs = [ go ];
          
          meta = with pkgs.lib; {
            description = "Terraform provider for Nscale";
            homepage = "https://github.com/nscaledev/terraform-provider-nscale";
            license = licenses.asl20;
            maintainers = [ ];
          };
        };
      });
}