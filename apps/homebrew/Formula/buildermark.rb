class Buildermark < Formula
  desc "Buildermark desktop app (macOS) and CLI service manager (Linux)"
  homepage "https://buildermark.dev"
  version "1.0.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/Buildermark-#{version}-macos-arm64.dmg"
      sha256 "3a6c48c4e00f11dba2d86621e57759326668730ee2a8e8e37aa5a57d7c96cd12"
    end

    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/Buildermark-#{version}-macos-amd64.dmg"
      sha256 "a1189a68367cf767e8540d5109b5e54f2d74c57dedd5c56d62930d960e29627b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-#{version}-linux-arm64.tar.gz"
      sha256 "0dbccd2c85d19cc73f5d8180b23b7f9cd6e5e619d83b670a27e9f1239c0e4eee"
    end

    on_intel do
      url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-#{version}-linux-amd64.tar.gz"
      sha256 "1d8623ccd4a68e7b7dab5dd711bc5dd5cb8845321acfffa5059a3852c005a9ab"
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
