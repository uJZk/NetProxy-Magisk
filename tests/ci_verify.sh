#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out=${NETPROXY_CI_BUILD_DIR:-"$root/.agents/runs/ci-$$"}
mkdir -p "$out"
cd "$root/src/native/netproxy"
go test ./...
go vet ./...
go build -trimpath -o "$out/netproxyctl" ./cmd/netproxyctl
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o "$out/netproxyctl-android" ./cmd/netproxyctl
cd "$root"
sh -n src/module/customize.sh
sh -n .github/scripts/stage-module.sh
sh -n .github/scripts/package-module.sh
bash -n .github/scripts/build-core.sh
file "$out/netproxyctl-android" | grep -q 'ARM aarch64'
