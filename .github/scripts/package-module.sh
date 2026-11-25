#!/bin/sh
# Magisk installers can extract standard ZIP Deflate archives.
set -eu
test "$#" -eq 3
module_dir=$(CDPATH= cd -- "$1" && pwd)
mkdir -p "$2"
output_dir=$(CDPATH= cd -- "$2" && pwd)
archive=$output_dir/$3
test -f "$module_dir/module.prop"
test ! -e "$archive"
(cd "$module_dir" && zip -q -9 -r "$archive" . -x module.prop && zip -q -0 "$archive" module.prop)
unzip -tq "$archive"
