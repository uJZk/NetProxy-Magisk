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
export async function submitFile<T = unknown>(args: string[], content: string) {
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
  } finally {
    await shell("rm -f " + quote(path));
  }
}
export type Config = { content: string; revision: string; status: string };
