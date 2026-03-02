cask "buildermark" do
  version "1.0.0"

  on_arm do
    sha256 "REPLACE_WITH_ARM64_DMG_SHA256"
    url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-macos-arm64.dmg"
  end

  on_intel do
    sha256 "REPLACE_WITH_AMD64_DMG_SHA256"
    url "https://github.com/buildermark/buildermark/releases/download/v#{version}/buildermark-macos-amd64.dmg"
  end

  name "Buildermark"
  desc "Menu bar app that runs and manages local Buildermark services"
  homepage "https://buildermark.dev"

  app "Buildermark.app"

  zap trash: [
    "~/Library/Application Support/Buildermark",
    "~/Library/Preferences/dev.buildermark.Buildermark.plist",
  ]
end
