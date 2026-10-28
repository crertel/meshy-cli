{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "meshy";
  version = "0.2.0";

  src = lib.cleanSource ./.;

  vendorHash = "sha256-tFFy7I2Tj0kXqh+gs694Bc02K2JCzQhigeYEhUi+K5E=";

  subPackages = [ "cmd/meshy" ];

  # Pure-Go sqlite (modernc); no CGO needed for a portable binary.
  env.CGO_ENABLED = "0";

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "CLI for the Meshy AI 3D generation API";
    homepage = "https://github.com/crertel/meshy-cli";
    mainProgram = "meshy";
    platforms = lib.platforms.unix;
  };
}
