#!/bin/sh
set -eu

version=v0.11.0
dir=${1:?usage: install-shellcheck.sh <dir>}

case "$(uname -s)/$(uname -m)" in
Darwin/arm64)
	platform=darwin.aarch64
	sum=56affdd8de5527894dca6dc3d7e0a99a873b0f004d7aabc30ae407d3f48b0a79
	;;
Darwin/x86_64)
	platform=darwin.x86_64
	sum=3c89db4edcab7cf1c27bff178882e0f6f27f7afdf54e859fa041fca10febe4c6
	;;
Linux/aarch64 | Linux/arm64)
	platform=linux.aarch64
	sum=12b331c1d2db6b9eb13cfca64306b1b157a86eb69db83023e261eaa7e7c14588
	;;
Linux/x86_64)
	platform=linux.x86_64
	sum=8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198
	;;
*)
	echo "no pinned shellcheck build for $(uname -s)/$(uname -m)" >&2
	exit 1
	;;
esac

if [ -x "$dir/shellcheck" ] && "$dir/shellcheck" --version | grep -qx "version: ${version#v}"; then
	exit 0
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
archive=shellcheck-$version.$platform.tar.xz
curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$archive" \
	"https://github.com/koalaman/shellcheck/releases/download/$version/$archive"

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
else
	actual=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
fi
if [ "$actual" != "$sum" ]; then
	echo "checksum mismatch for $archive: expected $sum, got $actual" >&2
	exit 1
fi

tar -xJf "$tmp/$archive" -C "$tmp"
mkdir -p "$dir"
mv "$tmp/shellcheck-$version/shellcheck" "$dir/shellcheck"
