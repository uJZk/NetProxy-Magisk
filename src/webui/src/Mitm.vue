<script setup lang="ts">
import { ref, onMounted } from "vue";
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
    ...site.value,
    routes: rows.value.map((r) => ({ path: r.path, json: JSON.parse(r.body) })),
  };
}
async function read() {
  const data = await command<Config>("config", "read", "mitm");
  const s = JSON.parse(data.content);
  fill({ ...s, target_ip: s.target_ip || "192.2.0.1", private_key: "" });
  revision.value = data.revision;
  message.value = JSON.parse(data.status).message;
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
async function save() {
  const s = draft();
  await submitFile(
    ["config", "apply", "--revision", revision.value, "mitm"],
    JSON.stringify({
      enabled: s.enabled,
      target_ip: s.target_ip,
      certificate: s.certificate,
      private_key: s.private_key,
      routes: s.routes,
    }),
  );
  await read();
  message.value = "MITM 设置已保存";
}
async function backup() {
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
async function importFile(e: Event) {
  const el = e.target as HTMLInputElement;
  const f = el.files?.[0];
  el.value = "";
  if (!f) return;
  await run(async () => {
    if (f.size > 262144) throw Error("备份超过 256 KiB");
    const result = importSite(await f.text(), draft());
    fill(result.site);
    message.value = result.summary;
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
        >导入本项目或 ESP32 备份<input
          type="file"
          accept=".json,.yaml,.yml,.txt"
          @change="importFile"
      /></label>
      <VanButton @click="run(backup)">导出备份（含私钥）</VanButton>
      <VanButton type="primary" @click="run(save)">保存设置</VanButton>
    </fieldset>
    <p role="status">{{ message }}</p>
    <VanButton :disabled="busy" @click="run(read)">重新读取</VanButton>
  </section>
</template>
