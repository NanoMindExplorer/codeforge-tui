# Homebrew formula (tap: NanoMindExplorer/codeforge or copy into a personal tap)
#   brew install --build-from-source Formula/codeforge.rb
#   brew install NanoMindExplorer/tap/codeforge   # when published
class Codeforge < Formula
  desc "Terminal AI coding companion (multi-provider agent + GitHub)"
  homepage "https://github.com/NanoMindExplorer/codeforge"
  version "1.9.1"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/NanoMindExplorer/codeforge/releases/download/v#{version}/codeforge_#{version}_darwin_arm64.tar.gz"
      # sha256 "REPLACE_ON_RELEASE"
    end
    on_intel do
      url "https://github.com/NanoMindExplorer/codeforge/releases/download/v#{version}/codeforge_#{version}_darwin_amd64.tar.gz"
      # sha256 "REPLACE_ON_RELEASE"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/NanoMindExplorer/codeforge/releases/download/v#{version}/codeforge_#{version}_linux_arm64.tar.gz"
      # sha256 "REPLACE_ON_RELEASE"
    end
    on_intel do
      url "https://github.com/NanoMindExplorer/codeforge/releases/download/v#{version}/codeforge_#{version}_linux_amd64.tar.gz"
      # sha256 "REPLACE_ON_RELEASE"
    end
  end

  def install
    bin.install "codeforge"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/codeforge version")
  end
end
