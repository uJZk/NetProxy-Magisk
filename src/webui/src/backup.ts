import { parseDocument } from "yaml";

export type Route = { path: string; json: unknown };
export interface Site {
  enabled: boolean;
  target_ip: string;
  certificate: string;
  private_key: string;
  routes: Route[];
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw Error("备份结构无效");
  return value as Record<string, unknown>;
}
function string(value: unknown) {
  if (typeof value !== "string") throw Error("备份字段应为文本");
  return value;
}
function routes(input: unknown, esp: boolean): Route[] {
  if (!Array.isArray(input) || input.length > 8) throw Error("接口列表无效");
  return input.map((item, index) => {
    const route = record(item);
    if (!("json" in route)) throw Error("接口缺少 JSON");
    let json = route.json;
    if (esp && typeof json === "string") {
      try { json = JSON.parse(json); }
      catch { throw Error(`第 ${index + 1} 条 HTTPS 接口的响应不是有效 JSON`); }
    } else if (esp && (json === null || typeof json !== "object")) {
      throw Error(`第 ${index + 1} 条 HTTPS 接口的 json 需要文本、对象或数组`);
    }
    return { path: string(route.path).trim(), json };
  });
}

export function importSite(
  text: string,
  current: Site,
): { site: Site; summary: string } {
  const doc = parseDocument(text, { schema: "core", uniqueKeys: true });
  if (doc.errors.length) throw Error("备份 YAML 或 JSON 语法无效");
  const data = record(doc.toJS({ maxAliasCount: 0 }));
  if (data.version !== undefined && data.version !== 1)
    throw Error("备份版本不受支持");
  if (
    data.format === "netproxy-mitm" ||
    ("enabled" in data && "routes" in data)
  ) {
    const site = data.format ? record(data.mitm) : data;
    if (typeof site.enabled !== "boolean") throw Error("备份缺少 MITM 总开关");
    return {
      site: {
        enabled: site.enabled,
        target_ip: string(site.target_ip ?? "192.2.0.1"),
        certificate: string(site.certificate),
        private_key: string(site.private_key),
        routes: routes(site.routes ?? [], false),
      },
      summary: "已载入本项目 MITM 设置，保存后生效。",
    };
  }
  const site = { ...current };
  const changed: string[] = [];
  if ("h2_site" in data) {
    const h2 = record(data.h2_site);
    const cert = string(h2.cert ?? "").trim();
    const key = string(h2.key ?? "").trim();
    if (cert || key) {
      site.certificate = cert;
      site.private_key = key;
      if (!site.certificate || !site.private_key)
        throw Error("备份需同时携带证书和私钥");
      changed.push("证书和私钥");
    }
  }
  if ("h2_routes" in data) {
    site.routes = routes(data.h2_routes ?? [], true);
    changed.push(`HTTPS 接口（${site.routes.length} 条）`);
  }
  if (
    !["h2_routes", "h2_site", "wifi", "dns", "dns_blacklist", "blocklist"].some(
      (key) => key in data,
    )
  )
    throw Error("无法识别备份来源");
  return {
    site,
    summary: changed.length
      ? "待保存：" + changed.join("、") + "。其他设置保持当前值。"
      : "备份没有可导入的证书或接口。",
  };
}
