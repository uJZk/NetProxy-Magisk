import { ctlJson, shell } from "./exec";
import { decodeCtlResult } from "./contract";

export async function command<T>(...args: string[]): Promise<T> {
  const res = await ctlJson<T>(args, 180000);
  if (!res.ok) throw Error(res.message);
  return res.data as T;
}
function quote(value: string) {
  return "'" + value.replace(/'/g, "'\\''") + "'";
}
async function withTempFile<T>(content: string, use: (path: string) => Promise<T>) {
  const tmp = await shell(
    "umask 077 && mktemp /data/local/tmp/netproxy.XXXXXXXX",
  );
  const path = tmp.out.trim();
  if (tmp.code || !/^\/data\/local\/tmp\/netproxy\.[a-zA-Z0-9]+$/.test(path))
    throw Error("无法创建暂存文件");
  try {
    const bytes = new TextEncoder().encode(content);
    for (let offset = 0; offset < bytes.length; offset += 24576) {
      const encoded = btoa(
        String.fromCharCode(...bytes.subarray(offset, offset + 24576)),
      );
      const result = await shell(
        `printf %s ${quote(encoded)} | base64 -d >> ${quote(path)}`,
      );
      if (result.code) throw Error("写入暂存文件失败");
    }
    return await use(path);
  } finally {
    await shell("rm -f " + quote(path));
  }
}
export async function submitFile<T = unknown>(args: string[], content: string) {
  return withTempFile(content, async (path) => {
    const result = decodeCtlResult<T>(
      await shell(
        [
          "/data/adb/modules/netproxy/netproxyctl",
          "--json",
          "--timeout",
          "180s",
          ...args,
          path,
        ]
          .map(quote)
          .join(" "),
      ),
    );
    if (!result.ok) throw Error(result.message);
    return result.data as T;
  });
}

export async function saveMitmBackup(content: string): Promise<string> {
  return withTempFile(content, async (path) => {
    const result = await shell(`umask 077
mkdir -p /sdcard/Download || exit 1
dir=$(mktemp -d /sdcard/Download/netproxy-XXXXXXXX) || exit 1
if cp ${quote(path)} "$dir/.partial" && mv "$dir/.partial" "$dir/netproxy-mitm.json"; then
  printf '%s' "$dir/netproxy-mitm.json"
else
  rm -f "$dir/.partial"
  rmdir "$dir"
  exit 1
fi`);
    if (result.code) throw Error("无法写入备份到下载目录");
    return result.out.trim();
  });
}

export type Config = { content: string; revision: string; status: string };
