class BuildermarkLinux < Formula
  desc "Buildermark CLI for running and managing the local service on Linux"
  homepage "https://buildermark.dev"
  version "1.0.0"
  license "MIT"

  depends_on :linux

  on_linux do
    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-#{version}-linux-amd64.tar.gz"
      sha256 "1d8623ccd4a68e7b7dab5dd711bc5dd5cb8845321acfffa5059a3852c005a9ab"
    end

    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-#{version}-linux-arm64.tar.gz"
      sha256 "0dbccd2c85d19cc73f5d8180b23b7f9cd6e5e619d83b670a27e9f1239c0e4eee"
    end
  end

  def install
    bin.install "buildermark"
  end

  test do
    assert_match "buildermark", shell_output("#{bin}/buildermark version")
  end
end
