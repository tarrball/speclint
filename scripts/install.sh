#!/bin/sh
# Installs slint, the speclint command, from a GitHub release on Linux or macOS:
#
#   curl -fsSL https://raw.githubusercontent.com/tarrball/speclint/main/scripts/install.sh | sh
#
# SLINT_VERSION is the release to install, for example v0.4.0. The default is
# the latest release. SLINT_INSTALL_DIR is the directory for slint. The default is
# $HOME/.local/bin. The script does not use sudo. To update slint, run the
# script again.
set -eu

repo=tarrball/speclint
dir="${SLINT_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

command -v curl >/dev/null || fail "curl is necessary, but it is not on PATH"

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "this script works on Linux and macOS. On Windows, download slint.exe from https://github.com/$repo/releases" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "there is no release binary for $(uname -m)" ;;
esac

version="${SLINT_VERSION:-}"
if [ -z "$version" ]; then
	# GitHub sends /releases/latest to the page of the latest tag. The script
	# does not use the API, because the API has a rate limit.
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
		fail "cannot get the latest release from GitHub"
	version="${url##*/}"
	case "$version" in
	v*) ;;
	*) fail "cannot find the latest release; set SLINT_VERSION, for example SLINT_VERSION=v0.4.0" ;;
	esac
fi
case "$version" in
v*) ;;
*) fail "SLINT_VERSION is \"$version\"; use the tag of a release, for example v0.4.0" ;;
esac

name="slint_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading slint $version for $os/$arch"
curl -fsSL -o "$tmp/$name" "$base/$name" || fail "cannot download $base/$name"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "cannot download $base/checksums.txt"

# Compare the binary with its line in checksums.txt.
awk -v n="$name" '$2 == n' "$tmp/checksums.txt" >"$tmp/sum.txt"
[ -s "$tmp/sum.txt" ] || fail "checksums.txt has no line for $name"
if command -v sha256sum >/dev/null; then
	(cd "$tmp" && sha256sum -c sum.txt >/dev/null) || fail "the checksum of $name is not correct"
else
	(cd "$tmp" && shasum -a 256 -c sum.txt >/dev/null) || fail "the checksum of $name is not correct"
fi

# Copy the binary next to the old one, then rename it. A rename in one
# directory replaces the old binary in one step, so a hook that runs slint
# during an update gets the old or the new binary, never a part of one.
mkdir -p "$dir"
old=$("$dir/slint" version 2>/dev/null || true)
trap 'rm -rf "$tmp" "$dir/.slint.new"' EXIT
cp "$tmp/$name" "$dir/.slint.new"
chmod 755 "$dir/.slint.new"
# Run the new binary before it replaces the old one. It cannot run if, for
# example, the file system of the directory does not let programs run.
new=$("$dir/.slint.new" version) ||
	fail "the new binary does not run in $dir; set SLINT_INSTALL_DIR to another directory"
mv -f "$dir/.slint.new" "$dir/slint"
if [ -n "$old" ] && [ "$old" != "$new" ]; then
	echo "Updated $dir/slint from $old to $new"
else
	echo "Installed $new in $dir/slint"
fi

# The Claude Code hook runs the slint on PATH, so tell the user if that is not
# this one.
case ":$PATH:" in
*":$dir:"*)
	found=$(command -v slint || true)
	if [ "$found" != "$dir/slint" ]; then
		echo "Note: $found comes first on PATH. Remove it, or put $dir before it."
	fi
	;;
*)
	echo "Note: $dir is not on PATH. Add this line to your shell profile:"
	echo "  export PATH=\"$dir:\$PATH\""
	;;
esac
