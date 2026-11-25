#!/system/bin/sh
# Magisk, KernelSU and APatch extract this module into their staging directory.
SKIPUNZIP=1
umask 077
live=/data/adb/modules/netproxy
[ -d "$MODPATH" ] || abort "模块暂存目录不存在"
[ "$(cd "$MODPATH" && pwd -P)" != "$live" ] || [ ! -e "$live/config/module.json" ] || abort "升级需要独立暂存目录"
unzip -o "$ZIPFILE" -x 'META-INF/*' -d "$MODPATH" >/dev/null || abort "解压模块失败"
for binary in bin/netproxyctl bin/sing-box netproxyctl; do
  [ -s "$MODPATH/$binary" ] || abort "缺少 $binary"
done
if [ -d "$live/config" ] && [ "$MODPATH" != "$live" ]; then
  (
    mkdir -p /dev/netproxy || exit 1
    exec 9>/dev/netproxy/service.lock.flock
    # BusyBox flock supports nonblocking acquisition but has no -w option.
    waited=0
    until flock -n 9 9>&9; do
      [ "$waited" -lt 20 ] || { ui_print "等待服务锁超时：/dev/netproxy/service.lock.flock"; exit 1; }
      sleep 1
      waited=$((waited + 1))
    done
    for dir in config logs data cores; do
      [ -d "$live/$dir" ] || continue
      mkdir -p "$MODPATH/$dir" && cp -a "$live/$dir/." "$MODPATH/$dir/" || { ui_print "复制旧版 $dir 目录失败"; exit 1; }
    done
    if [ -f "$live/cores/selected" ]; then
      cp -p "$live/bin/sing-box" "$MODPATH/bin/sing-box" || exit 1
    fi
    for channel in stable testing; do
      if [ -f "$live/cores/$channel" ] && [ "$live/bin/sing-box" -ef "$live/cores/$channel" ]; then
        ln -f "$MODPATH/cores/$channel" "$MODPATH/bin/sing-box" || exit 1
      fi
    done
  ) || abort "保留旧版用户数据失败"
fi
set_perm_recursive "$MODPATH" 0 0 0755 0644
set_perm_recursive "$MODPATH/config" 0 0 0700 0600
for dir in logs data cores; do
  [ ! -d "$MODPATH/$dir" ] || set_perm_recursive "$MODPATH/$dir" 0 0 0700 0600
done
for binary in bin/netproxyctl bin/sing-box netproxyctl service.sh uninstall.sh cores/stable cores/testing; do
  [ ! -f "$MODPATH/$binary" ] || set_perm "$MODPATH/$binary" 0 0 0700
done
NETPROXY_MODULE_DIR="$MODPATH" "$MODPATH/bin/netproxyctl" config upgrade-snapshot >/dev/null || abort "旧配置迁移失败"
version=$("$MODPATH/bin/sing-box" version | head -n 1)
case "$version" in
  *-alpha.*|*-beta.*|*-rc.*) channel=testing ;;
  *) channel=stable ;;
esac
mkdir -p "$MODPATH/cores" || abort "无法准备核心目录"
if [ ! -e "$MODPATH/cores/$channel" ]; then
  ln "$MODPATH/bin/sing-box" "$MODPATH/cores/$channel" || abort "无法保存内置核心"
fi
ui_print "NetProxy 已安装，重启后生效。"
