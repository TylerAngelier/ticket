#!/usr/bin/env bash
# Publish ticket packages to the Homebrew tap.
#
# Usage:
#   ./scripts/publish-homebrew.sh <version> <sha256>            # clone, update, push
#   DRY_RUN=1 ./scripts/publish-homebrew.sh <version> <sha256>  # write formulas locally
#
# Requires: TAP_GITHUB_TOKEN environment variable (unless DRY_RUN=1)
#
# Two-formula layout:
#   ticket-core  just the `tk` bash script
#   ticket       everything: tk, bash plugins, and the compiled Go plugin

set -euo pipefail

VERSION="${1#v}"
SHA256="$2"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TAP_REPO="${TAP_REPO:-TylerAngelier/homebrew-tap}"
SOURCE_REPO="${SOURCE_REPO:-github.com/TylerAngelier/ticket}"
DRY_RUN="${DRY_RUN:-0}"

if [[ -z "$VERSION" || -z "$SHA256" ]]; then
    echo "Usage: $0 <version> <sha256>" >&2
    exit 2
fi

# Generate both formulas into a directory.
generate_formulas() {
    local out_dir="$1"
    mkdir -p "$out_dir"

    # --- ticket-core: the bash script alone -------------------------------
    cat > "$out_dir/ticket-core.rb" << EOF
class TicketCore < Formula
  desc "Minimal ticket tracking in bash (core only)"
  homepage "https://${SOURCE_REPO}"
  url "https://${SOURCE_REPO}/archive/refs/tags/v${VERSION}.tar.gz"
  sha256 "${SHA256}"
  license "MIT"

  def install
    bin.install "ticket" => "tk"
    bin.install_symlink "tk" => "ticket"
  end

  test do
    system "#{bin}/tk", "help"
  end
end
EOF

    # --- ticket: core + bash plugins + compiled Go plugin -----------------
    cat > "$out_dir/ticket.rb" << EOF
class Ticket < Formula
  desc "Minimal ticket tracking with hierarchical list view"
  homepage "https://${SOURCE_REPO}"
  url "https://${SOURCE_REPO}/archive/refs/tags/v${VERSION}.tar.gz"
  sha256 "${SHA256}"
  license "MIT"

  depends_on "go" => :build

  def install
    # Core CLI + alias
    bin.install "ticket" => "tk"
    bin.install_symlink "tk" => "ticket"

    # Compiled Go plugin backing ls/list/ready/blocked
    system "go", "build", "-trimpath", "-o", "ticket-ls", "./cmd/ticket-ls"
    bin.install "ticket-ls"
    ["ticket-list", "ticket-ready", "ticket-blocked"].each do |name|
      bin.install_symlink "ticket-ls" => name
    end

    # Bash plugins (regular files only; symlinks were release-time aliases)
    Dir["plugins/ticket-*"].each do |plugin|
      bin.install plugin unless File.symlink?(plugin)
    end
  end

  test do
    system "#{bin}/tk", "help"
    assert_match "tk-plugin-version:",
                 shell_output("#{bin}/ticket-ls --tk-describe")
  end
end
EOF
}

main() {
    echo "Homebrew publish (v${VERSION}, tap ${TAP_REPO})"

    if [[ "$DRY_RUN" == "1" ]]; then
        local out="/tmp/tap-dryrun"
        rm -rf "$out"
        generate_formulas "$out/Formula"
        echo "DRY RUN: formulas written to ${out}/Formula (nothing pushed)"
        ls -la "$out/Formula"
        exit 0
    fi

    [[ -n "${TAP_GITHUB_TOKEN:-}" ]] || {
        echo "Error: TAP_GITHUB_TOKEN not set (or use DRY_RUN=1)" >&2
        exit 1
    }
    # Strip stray whitespace/quotes that can sneak in when pasting the secret
    local token
    token="$(printf '%s' "${TAP_GITHUB_TOKEN}" | tr -d '[:space:]"')"

    local tap_dir="/tmp/homebrew-tap"
    rm -rf "$tap_dir"
    git clone --depth 1 \
        "https://x-access-token:${token}@github.com/${TAP_REPO}.git" \
        "$tap_dir"

    generate_formulas "$tap_dir/Formula"

    cd "$tap_dir"
    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"
    git add Formula/

    if git diff --cached --quiet; then
        echo "No changes to publish"
        exit 0
    fi

    git commit -m "ticket v${VERSION}"
    git push

    echo "Formulas published to ${TAP_REPO}"
}

main "$@"
