class BuildermarkLinux < Formula
  desc "Buildermark CLI for running and managing the local service on Linux"
  homepage "https://buildermark.dev"
  version "1.0.0"
  license "MIT"

  depends_on :linux

  on_linux do
    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-linux-amd64.tar.gz"
      sha256 "REPLACE_WITH_AMD64_TAR_SHA256"
    end

    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-linux-arm64.tar.gz"
      sha256 "REPLACE_WITH_ARM64_TAR_SHA256"
    end
  end

  def install
    bin.install "buildermark"
  end

  test do
    assert_match "buildermark", shell_output("#{bin}/buildermark version")
  end
end
