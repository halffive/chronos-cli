#!/bin/sh
# Installs the chronos CLI from GitHub Releases:
#
#   curl -fsSL https://chronos.sh/install.sh | sh
#   curl -fsSL https://chronos.sh/install.sh | sh -s -- v0.1.0
#
# Verifies checksums.txt and installs to $CHRONOS_INSTALL_DIR, default
# ~/.local/bin. Never edits a shell profile.
set -eu

releases=https://github.com/halffive/chronos-cli/releases

fail() {
	echo "chronos install: $*" >&2
	exit 1
}

detect_os() {
	case $(uname -s) in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) fail "unsupported OS $(uname -s); download a build from $releases" ;;
	esac
}

detect_arch() {
	case $(uname -m) in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture $(uname -m); download a build from $releases" ;;
	esac
}

check_version() {
	case $1 in
	*[!0-9A-Za-z.+-]*) fail "invalid version $1" ;;
	v[0-9]*) ;;
	*) fail "invalid version $1" ;;
	esac
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "install sha256sum or shasum to verify the download"
	fi
}

download() {
	curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"
}

main() {
	version=${1:-}
	install_dir=${CHRONOS_INSTALL_DIR:-$HOME/.local/bin}
	os=$(detect_os)
	arch=$(detect_arch)

	if [ -z "$version" ]; then
		base=$releases/latest/download
	else
		case $version in
		v*) ;;
		*) version=v$version ;;
		esac
		check_version "$version"
		base=$releases/download/$version
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' HUP INT TERM

	download "$base/checksums.txt" "$tmp/checksums.txt" ||
		fail "could not download $base/checksums.txt"
	line=$(grep -E "^[0-9a-f]{64}  chronos_[^ /]+_${os}_${arch}\.tar\.gz$" "$tmp/checksums.txt") ||
		fail "no ${os}_${arch} archive listed in $base/checksums.txt"
	expected=${line%% *}
	archive=${line##* }

	download "$base/$archive" "$tmp/$archive" ||
		fail "could not download $base/$archive"
	actual=$(sha256 "$tmp/$archive")
	[ "$actual" = "$expected" ] ||
		fail "checksum mismatch for $archive: expected $expected, got $actual"

	tar -xzf "$tmp/$archive" -C "$tmp" chronos
	mkdir -p "$install_dir"
	install_dir=$(CDPATH='' cd -- "$install_dir" && pwd)
	cp "$tmp/chronos" "$install_dir/.chronos.$$"
	chmod 755 "$install_dir/.chronos.$$"
	mv -f "$install_dir/.chronos.$$" "$install_dir/chronos"

	installed=${archive#chronos_}
	installed=${installed%_"${os}"_"${arch}".tar.gz}
	echo "Installed chronos $installed to $install_dir/chronos"

	case ":$PATH:" in
	*":$install_dir:"*) ;;
	*)
		echo
		echo "$install_dir is not on your PATH. Add it to your shell profile:"
		echo "  export PATH=\"$install_dir:\$PATH\""
		;;
	esac
}

main "$@"
