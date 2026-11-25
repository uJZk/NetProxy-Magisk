#!/bin/sh
set -eu
source_dir=$1
stage=$2
mkdir -p "$stage/bin" "$stage/config/singbox" "$stage/webroot"
cp LICENSE NOTICE "$stage/"
cp -a "$source_dir/META-INF" "$stage/"
for file in module.prop customize.sh netproxyctl service.sh uninstall.sh; do cp "$source_dir/$file" "$stage/"; done
cp "$source_dir/bin/sing-box" "$source_dir/bin/netproxyctl" "$stage/bin/"
cp -r "$source_dir/cores" "$stage/"
cp "$source_dir/config/module.json" "$stage/config/"
cp "$source_dir/webroot/index.html" "$stage/webroot/"
cp -r "$source_dir/webroot/netproxy" "$stage/webroot/"
cp -r "$source_dir/webroot/sing-box-dashboard" "$stage/webroot/"
