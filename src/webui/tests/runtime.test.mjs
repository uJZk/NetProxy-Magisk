import { test } from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  rmSync,
  readFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { checkBuild } from "../scripts/check-build.mjs";
import { decodeCtlResult } from "../src/contract.ts";
test('构建产物引用本地非空资源', t=>{
 const dir=mkdtempSync(join(tmpdir(),'netproxy-web-'))
 t.after(()=>rmSync(dir,{recursive:true,force:true}))
 writeFileSync(join(dir,'index.html'),'<script src="./app.js"></script>')
 writeFileSync(join(dir,'hotspot.html'),'<a href="./index.html#mitm">MITM</a>')
 assert.throws(()=>checkBuild(dir))
 writeFileSync(join(dir,'app.js'),'')
 assert.throws(()=>checkBuild(dir))
 writeFileSync(join(dir,'app.js'),'export {}')
 checkBuild(dir)
})
test("JSON 与进程退出状态必须同时成功，结构化失败保留", () => {
  const success = {
    schema: 1,
    ok: true,
    code: "service.status",
    message: "服务状态",
    data: { state: "ready" },
  };
  assert.deepEqual(
    decodeCtlResult({ out: JSON.stringify(success), err: "", code: 0 }),
    success,
  );
  assert.equal(
    decodeCtlResult({
      out: JSON.stringify(success),
      err: "terminated",
      code: 1,
    }).ok,
    false,
  );
  const failure = {
    ...success,
    ok: false,
    code: "subscription.runtime_sync_failed",
    message: "运行时同步失败",
  };
  assert.deepEqual(
    decodeCtlResult({ out: JSON.stringify(failure), err: "extra", code: 1 }),
    failure,
  );
  assert.equal(
    decodeCtlResult({ out: '{"schema":2}', err: "", code: 0 }).code,
    "transport.invalid_json",
  );
  assert.equal(
    decodeCtlResult({ out: "", err: "denied", code: 1 }).message,
    "denied",
  );
});

import { importSite } from "../src/backup.ts";
const current = {
  enabled: false,
  target_ip: "127.0.0.2",
  certificate: "existing",
  private_key: "saved",
  routes: [{ path: "/old", json: {} }],
};
test("ESP32 导入保留缺省段，显式空列表清空接口", () => {
  const backup = `version: 1\nwifi: {}\nh2_site:\n  domain: example.test\nh2_routes:\n  - path: /hello\n    json: '{"ok":true}'\n`;
  const { site } = importSite(backup, current);
  assert.deepEqual(site, {
    ...current,
    routes: [{ path: "/hello", json: { ok: true } }],
  });
  assert.deepEqual(
    importSite("version: 1\nh2_routes: []", current).site.routes,
    [],
  );
  assert.deepEqual(
    importSite("version: 1\nh2_site:\n  domain: test.local", current).site,
    current,
  );
  assert.throws(() => importSite("version: 2\nh2_routes: []", current));
});
test("本项目备份往返恢复开关地址证书和接口", () => {
  const backup = JSON.stringify({
    format: "netproxy-mitm",
    version: 1,
    mitm: current,
  });
  assert.deepEqual(
    importSite(backup, { ...current, routes: [] }).site,
    current,
  );
});
test("ESP32 接受嵌套 JSON、空证书对和空接口段", () => {
  const sample = "h2_site: {cert: '', key: ''}\nh2_routes:\n  - path: /test\n    json: {ready: true, items: [1, 2]}";
  assert.deepEqual(importSite(sample, current).site, {
    ...current, routes: [{path: "/test", json: {ready: true, items: [1, 2]}}]
  });
  assert.deepEqual(importSite("h2_routes:", current).site.routes, []);
  assert.throws(() => importSite("h2_site: {cert: certificate}", current));
});
