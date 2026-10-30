#!/bin/sh
# The tag stays local. GoReleaser creates it on GitHub at GITHUB_SHA through
# the release. A failed module proxy fetch only warns, because the release
# already exists.
set -eu

version=${1:?usage: release.sh <version>, for example 0.1.0}
if ! echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
	echo "invalid version $version: pass it without the v prefix, for example 0.1.0" >&2
	exit 1
fi
tag=v$version

git tag "$tag" "$GITHUB_SHA"
GORELEASER_CURRENT_TAG=$tag go run "$GORELEASER" release --clean

if ! GOPROXY=https://proxy.golang.org go list -m "chronos.sh/cli@$tag"; then
	echo "::warning::$tag is released, but the module proxy could not fetch it. Retry with: GOPROXY=https://proxy.golang.org go list -m chronos.sh/cli@$tag"
fi
