#!/usr/bin/env bash
# Arguments are a clean reF1nd checkout and the verified source tag.
set -euo pipefail
checkout=$1
tag=$2
test "$(git -C "$checkout" rev-parse HEAD)" = "$(git -C "$checkout" rev-parse "refs/tags/$tag^{commit}")"
export GOOS=android GOARCH=arm64 CGO_ENABLED=0
export TAGS="$(jq -r '.singBox.buildTags|join(",")' .github/resources.json)"
make -C "$checkout" build VERSION="${tag#v}"
file "$checkout/sing-box" | grep -q 'ARM aarch64'
go version -m "$checkout/sing-box" | grep -F "${tag#v}"
go version -m "$checkout/sing-box" | grep -F with_ebpf
