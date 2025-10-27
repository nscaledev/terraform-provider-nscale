{
  description = "Development environment for terraform-provider-nscale";

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
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go 1.25 (using the latest available Go version)
            go_1_25

            # Development tools
            golangci-lint
            terraform

            # Optional but useful tools
            gopls
            gotools
            go-tools
          ];

          shellHook = ''
            echo "terraform-provider-nscale development environment"
            echo "Go version: $(go version)"
            echo "Terraform version: $(terraform version | head -n1)"
            echo "golangci-lint version: $(golangci-lint version)"
          '';
        };
      }
    );
}
