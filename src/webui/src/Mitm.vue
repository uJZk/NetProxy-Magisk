<script setup lang="ts">
import { ref, watch, onBeforeUnmount, onMounted } from "vue";
import {
  Button as VanButton,
  Field as VanField,
  Switch as VanSwitch,
} from "vant";
import { command, submitFile, type Config } from "./control";
import { importSite, type Site } from "./backup";

const site = ref<Site>({
  enabled: false,
  target_ip: "192.2.0.1",
  certificate: "",
  private_key: "",
  routes: [],
});
const revision = ref(""),
  message = ref(""),
  busy = ref(false);
const rows = ref<{ path: string; body: string }[]>([]);
function fill(s: Site) {
  site.value = s;
  rows.value = (s.routes ?? []).map((r) => ({
    path: r.path,
    body: JSON.stringify(r.json, null, 2),
  }));
}
function draft(): Site {
  return {
    enabled: site.value.enabled,
    target_ip: site.value.target_ip,
    certificate: site.value.certificate,
    private_key: site.value.private_key,
    routes: rows.value.map((r) => ({ path: r.path, json: JSON.parse(r.body) })),
  };
}
async function read() {
  const data = await command<Config>("config", "read", "mitm");
  const s = JSON.parse(data.content);
  fill({ ...s, target_ip: s.target_ip || "192.2.0.1", private_key: "" });
  persisted = JSON.stringify(draft());
  revision.value = data.revision;
  const status = JSON.parse(data.status);
  message.value = status.state === "error" ? status.message : "";
}
async function run(fn: () => Promise<void>) {
  busy.value = true;
  try {
    await fn();
  } catch (e) {
    message.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}
let persisted = "";
let timer: ReturnType<typeof setTimeout> | undefined;
let queue: Promise<void> = Promise.resolve();

function flush() {
  clearTimeout(timer);
  queue = queue.catch(() => {}).then(async () => {
    if (!revision.value) return;
    const s = draft();
    const payload = JSON.stringify(s);
    if (payload === persisted) return;
    const saved = await submitFile<{ revision: string }>(
      ["config", "apply", "--revision", revision.value, "mitm"], payload,
    );
    revision.value = saved.revision;
    persisted = payload;
    if (s.private_key && site.value.private_key === s.private_key &&
        site.value.certificate === s.certificate) {
      persisted = JSON.stringify({ ...s, private_key: "" });
      site.value.private_key = "";
    }
    message.value = "";
  });
  return queue;
}
function saveChanges() {
  void flush().catch((error: unknown) => {
    message.value = "未保存：" + (error instanceof SyntaxError
      ? "请补全接口的 JSON 响应" : error instanceof Error ? error.message : String(error));
  });
}
watch([site, rows], () => {
  if (!revision.value) return;
  clearTimeout(timer);
  timer = setTimeout(saveChanges, 450);
}, { deep: true, flush: "sync" });
onBeforeUnmount(() => { if (revision.value) saveChanges(); });

async function backup() {
  await flush();
  const data = await command<Config>(
    "config",
    "read",
    "--include-private",
    "mitm",
  );
  const s = JSON.parse(data.content);
  const mitm: Site = {
    enabled: s.enabled,
    target_ip: s.target_ip || "192.2.0.1",
    certificate: s.certificate,
    private_key: s.private_key,
    routes: s.routes ?? [],
  };
  const url = URL.createObjectURL(
    new Blob(
      [JSON.stringify({ format: "netproxy-mitm", version: 1, mitm }, null, 2)],
      { type: "application/json" },
    ),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = "netproxy-mitm.json";
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}
const pastedBackup = ref("");
async function applyBackup(text: string) {
  if (new TextEncoder().encode(text).length > 262144) throw Error("备份超过 256 KiB");
  const result = importSite(text, draft());
  fill(result.site);
  await flush();
  message.value = result.summary;
  pastedBackup.value = "";
}
async function importFile(e: Event) {
  const el = e.target as HTMLInputElement;
  const f = el.files?.[0];
  el.value = "";
  if (!f) return;
  await run(async () => {
    if (f.size > 262144) throw Error("备份超过 256 KiB");
    await applyBackup(await f.text());
  });
}
onMounted(() => run(read));
</script>
<template>
  <section>

    <fieldset :disabled="busy || !revision">
      <label>启用 MITM <VanSwitch v-model="site.enabled" /></label>
      <VanField v-model="site.target_ip" label="目标 IPv4" />
      <VanField
        v-model="site.certificate"
        type="textarea"
        label="证书链 PEM"
        autosize
      />
      <VanField
        v-model="site.private_key"
        type="textarea"
        label="私钥 PEM"
        placeholder="沿用同一证书时可留空"
      />
      <h3>HTTPS 自定义接口</h3>
      <div v-for="(row, i) in rows" :key="i">
        <VanField v-model="row.path" label="路径" /><VanField
          v-model="row.body"
          label="响应 JSON"
          type="textarea"
        /><VanButton @click="rows.splice(i, 1)">删除接口</VanButton>
      </div>
      <VanButton
        :disabled="rows.length >= 8"
        @click="rows.push({ path: '', body: '{}' })"
        >添加接口</VanButton
      >
      <h3>MITM 备份</h3>
      <label
        >导入备份<input
          type="file"
          @change="importFile"
      /></label>
      <details><summary>粘贴备份文本</summary>
        <VanField v-model="pastedBackup" type="textarea" label="YAML / JSON" autocomplete="off" />
        <VanButton :disabled="!pastedBackup.trim()" @click="run(() => applyBackup(pastedBackup))">导入并应用</VanButton>
      </details>
      <VanButton @click="run(backup)">导出备份（含私钥）</VanButton>
    </fieldset>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
