class Buildermark < Formula
  desc "Buildermark desktop app (macOS) and CLI service manager (Linux)"
  homepage "https://buildermark.dev"
  version "1.0.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-macos-arm64.dmg"
      sha256 "REPLACE_WITH_ARM64_DMG_SHA256"
    end

    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-macos-amd64.dmg"
      sha256 "REPLACE_WITH_AMD64_DMG_SHA256"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-linux-arm64.tar.gz"
      sha256 "REPLACE_WITH_ARM64_TAR_SHA256"
    end

    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-linux-amd64.tar.gz"
      sha256 "REPLACE_WITH_AMD64_TAR_SHA256"
    end
  end

  def install
    if OS.mac?
      app_bundle = Dir["*.app"].first
      odie "Buildermark.app was not found in the DMG" if app_bundle.nil?

      prefix.install app_bundle
      bin.install_symlink prefix/"#{app_bundle}/Contents/MacOS/Buildermark" => "buildermark"
    else
      bin.install "buildermark"
    end
  end

  def caveats
    return unless OS.mac?

    <<~EOS
      Buildermark.app was installed to:
        #{opt_prefix}/Buildermark.app

      To make it visible in Launchpad/Finder, copy it into /Applications:
        cp -R #{opt_prefix}/Buildermark.app /Applications/
    EOS
  end

  test do
    if OS.mac?
      assert_predicate prefix/"Buildermark.app", :exist?
    else
      assert_match "buildermark", shell_output("#{bin}/buildermark version")
    end
  end
end
