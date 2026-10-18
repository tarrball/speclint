#!/bin/sh
# Installs sv, the specval command, from a GitHub release on Linux or macOS:
#
#   curl -fsSL https://raw.githubusercontent.com/tarrball/specval/main/scripts/install.sh | sh
#
# SV_VERSION is the release to install, for example v0.1.0. The default is
# the latest release. SV_INSTALL_DIR is the directory for sv. The default is
# $HOME/.local/bin. The script does not use sudo.
set -eu

repo=tarrball/specval
dir="${SV_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

command -v curl >/dev/null || fail "curl is necessary, but it is not on PATH"

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "this script works on Linux and macOS. On Windows, download sv.exe from https://github.com/$repo/releases" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "there is no release binary for $(uname -m)" ;;
esac

version="${SV_VERSION:-}"
if [ -z "$version" ]; then
	# GitHub sends /releases/latest to the page of the latest tag. The script
	# does not use the API, because the API has a rate limit.
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
		fail "cannot get the latest release from GitHub"
	version="${url##*/}"
	case "$version" in
	v*) ;;
	*) fail "cannot find the latest release; set SV_VERSION, for example SV_VERSION=v0.1.0" ;;
	esac
fi
case "$version" in
v*) ;;
*) fail "SV_VERSION is \"$version\"; use the tag of a release, for example v0.1.0" ;;
esac

name="sv_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading sv $version for $os/$arch"
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

mkdir -p "$dir"
chmod 755 "$tmp/$name"
mv "$tmp/$name" "$dir/sv"
echo "Installed $("$dir/sv" version) in $dir/sv"

# The Claude Code hook runs the sv on PATH, so tell the user if that is not
# this one.
case ":$PATH:" in
*":$dir:"*)
	found=$(command -v sv || true)
	if [ "$found" != "$dir/sv" ]; then
		echo "Note: $found comes first on PATH. Remove it, or put $dir before it."
	fi
	;;
*)
	echo "Note: $dir is not on PATH. Add this line to your shell profile:"
	echo "  export PATH=\"$dir:\$PATH\""
	;;
esac
