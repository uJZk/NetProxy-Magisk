#!/usr/bin/env sh
# 向模块包追加 Android 平台辅助程序。
set -eu

test "$#" -eq 2
module="$(CDPATH= cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")"
test -s "$module"
test -s "$2"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir "$stage/bin"
cp "$2" "$stage/bin/android-platform.dex"
(
  cd "$stage"
  7z a -tzip -mx=0 "$module" bin/android-platform.dex
)
7z t "$module"
