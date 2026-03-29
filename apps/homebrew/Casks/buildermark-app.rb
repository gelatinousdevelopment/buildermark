cask "buildermark-app" do
  version "1.0.0"

  on_arm do
    sha256 "3a6c48c4e00f11dba2d86621e57759326668730ee2a8e8e37aa5a57d7c96cd12"
    url "https://github.com/buildermark/buildermark/releases/download/v#{version}/Buildermark-#{version}-macos-arm64.dmg"
  end

  on_intel do
    sha256 "a1189a68367cf767e8540d5109b5e54f2d74c57dedd5c56d62930d960e29627b"
    url "https://github.com/buildermark/buildermark/releases/download/v#{version}/Buildermark-#{version}-macos-amd64.dmg"
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
