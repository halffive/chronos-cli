#!/bin/sh
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
dist=$root/dist/release
real_curl=$(command -v curl)
real_uname=$(command -v uname)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

die() {
	echo "FAIL: $case_name: $*" >&2
	for f in "$work/out" "$work/err"; do
		[ -s "$f" ] && sed "s|^|  ${f##*/}: |" "$f" >&2
	done
	exit 1
}

case $("$real_uname" -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) echo "unsupported host OS" >&2 && exit 1 ;;
esac
case $("$real_uname" -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) echo "unsupported host architecture" >&2 && exit 1 ;;
esac

archive=$(cd "$dist" && ls chronos_*_"${os}"_"${arch}".tar.gz)
version=${archive#chronos_}
version=${version%_"${os}"_"${arch}".tar.gz}
commit=$(git -C "$root" rev-parse HEAD)

mkdir -p "$work/bin"
cat >"$work/bin/curl" <<EOF
#!/bin/sh
for arg; do
	shift
	case \$arg in
	https://github.com/halffive/chronos-cli/releases/*)
		echo "\$arg" >>"\$CURL_LOG"
		arg="file://\$FAKE_RELEASES/\${arg#https://github.com/halffive/chronos-cli/releases/}"
		;;
	https://* | http://*)
		echo "unexpected URL \$arg" >&2
		exit 1
		;;
	=https) arg='=file' ;;
	esac
	set -- "\$@" "\$arg"
done
exec "$real_curl" "\$@"
EOF
chmod +x "$work/bin/curl"

new_release() {
	for path in latest/download "download/v$version"; do
		mkdir -p "$work/releases/$path"
		cp "$dist/checksums.txt" "$dist/$archive" "$work/releases/$path/"
	done
}

run_install() {
	rm -rf "${work:?}/home" "$work/out" "$work/err" "$work/curl.log"
	mkdir -p "$work/home"
	touch "$work/curl.log"
	status=0
	env HOME="$work/home" PATH="$path_prefix$work/bin:$PATH" \
		FAKE_RELEASES="$work/releases" CURL_LOG="$work/curl.log" \
		sh "$root/install.sh" "$@" >"$work/out" 2>"$work/err" || status=$?
}

start() {
	case_name=$1
	rm -rf "$work/releases" "$work/custom"
	new_release
	path_prefix=
	unset CHRONOS_INSTALL_DIR
}

expect_success() {
	[ "$status" -eq 0 ] || die "exit $status, want 0"
}

expect_failure() {
	[ "$status" -ne 0 ] || die "exit 0, want a failure"
	grep -qF -- "$1" "$work/err" || die "stderr does not contain: $1"
	[ ! -e "$work/home/.local/bin/chronos" ] || die "chronos was installed"
}

expect_installed() {
	[ -x "$1" ] || die "$1 is not an executable file"
	json=$("$1" version --json)
	echo "$json" | grep -qF "\"version\":\"$version\"" || die "version --json: $json"
	echo "$json" | grep -qF "\"commit\":\"$commit\"" || die "version --json: $json"
	echo "$json" | grep -qE '"built_at":"[0-9]{4}-' || die "version --json: $json"
}

expect_downloads_from() {
	[ -s "$work/curl.log" ] || die "nothing was downloaded"
	if grep -vF "https://github.com/halffive/chronos-cli/releases/$1/" "$work/curl.log" >/dev/null; then
		die "downloads outside $1: $(cat "$work/curl.log")"
	fi
}

start "latest release into ~/.local/bin"
run_install
expect_success
expect_installed "$work/home/.local/bin/chronos"
expect_downloads_from latest/download
grep -qF "Installed chronos $version to $work/home/.local/bin/chronos" "$work/out" || die "no install message"
echo "ok - $case_name"

start "explicit version"
run_install "v$version"
expect_success
expect_installed "$work/home/.local/bin/chronos"
expect_downloads_from "download/v$version"
echo "ok - $case_name"

start "explicit version without the v prefix"
run_install "$version"
expect_success
expect_downloads_from "download/v$version"
echo "ok - $case_name"

start "CHRONOS_INSTALL_DIR with a trailing slash on PATH"
CHRONOS_INSTALL_DIR=$work/custom/
export CHRONOS_INSTALL_DIR
mkdir -p "$work/custom"
path_prefix=$work/custom:
run_install
expect_success
grep -qF "to $work/custom/chronos" "$work/out" || die "install path not normalized"
if grep -qF "not on your PATH" "$work/out"; then die "printed a PATH hint"; fi
echo "ok - $case_name"

start "relative CHRONOS_INSTALL_DIR"
CHRONOS_INSTALL_DIR=custom
export CHRONOS_INSTALL_DIR
work_real=$(cd "$work" && pwd)
cd "$work"
run_install
cd "$root"
expect_success
grep -qF "export PATH=\"$work_real/custom:" "$work/out" || die "PATH hint is not absolute"
echo "ok - $case_name"

start "CHRONOS_INSTALL_DIR override"
CHRONOS_INSTALL_DIR=$work/custom
export CHRONOS_INSTALL_DIR
run_install
expect_success
expect_installed "$work/custom/chronos"
[ ! -e "$work/home/.local/bin/chronos" ] || die "also installed to ~/.local/bin"
echo "ok - $case_name"

start "PATH hint when the directory is not on PATH"
run_install
expect_success
grep -qxF "  export PATH=\"$work/home/.local/bin:\$PATH\"" "$work/out" || die "no export PATH line"
echo "ok - $case_name"

start "no PATH hint when the directory is on PATH"
path_prefix=$work/home/.local/bin:
run_install
expect_success
if grep -qF "export PATH" "$work/out"; then
	die "printed a PATH hint"
fi
echo "ok - $case_name"

start "checksum mismatch"
printf x >>"$work/releases/latest/download/$archive"
run_install
expect_failure "checksum mismatch for $archive"
echo "ok - $case_name"

start "missing checksum file"
rm "$work/releases/latest/download/checksums.txt"
run_install
expect_failure "could not download https://github.com/halffive/chronos-cli/releases/latest/download/checksums.txt"
if grep -qF "$archive" "$work/curl.log"; then
	die "downloaded the archive without a checksum file"
fi
echo "ok - $case_name"

start "unknown architecture"
cat >"$work/bin/uname" <<EOF
#!/bin/sh
if [ "\${1:-}" = -m ]; then
	echo riscv64
else
	exec "$real_uname" "\$@"
fi
EOF
chmod +x "$work/bin/uname"
run_install
rm "$work/bin/uname"
expect_failure "unsupported architecture riscv64"
[ ! -s "$work/curl.log" ] || die "downloaded on an unsupported architecture"
echo "ok - $case_name"
