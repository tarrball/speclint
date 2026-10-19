#!/bin/sh
# Builds the release binaries of slint into dist/, and writes their SHA-256
# checksums. The first argument is the version, for example v0.1.0. The
# binaries print it with `slint version`.
set -eu

version="${1:?usage: scripts/dist.sh <version>}"
rm -rf dist
mkdir dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os="${target%/*}"
	arch="${target#*/}"
	name="slint_${version}_${os}_${arch}"
	[ "$os" = windows ] && name="$name.exe"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
		go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/$name" ./cmd/slint
done
cd dist
if command -v sha256sum >/dev/null; then
	sha256sum slint_* >checksums.txt
else
	shasum -a 256 slint_* >checksums.txt
fi
