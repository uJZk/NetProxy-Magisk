<script setup lang="ts">
import { ref, watch, onBeforeUnmount, onMounted } from "vue";
import {
  Button as VanButton,
  Field as VanField,
  Switch as VanSwitch,
} from "vant";
import { command, saveMitmBackup, submitFile, type Config } from "./control";
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
  errorMessage = ref(""),
  saveError = ref(""),
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
function showSaved(data: Config) {
  const s = JSON.parse(data.content);
  fill({ ...s, target_ip: s.target_ip || "192.2.0.1" });
  persisted = JSON.stringify(draft());
  revision.value = data.revision;
  const status = JSON.parse(data.status);
  errorMessage.value = status.state === "error" ? status.message : "";
}
function readSettings() {
  return command<Config>("config", "read", "--include-private", "mitm");
}
async function read() {
  showSaved(await readSettings());
}
function applySettings(payload: string) {
  if (!revision.value) throw Error("缺少配置版本，请重新打开 MITM 页面");
  return submitFile<{ revision: string }>(
    ["config", "apply", "--revision", revision.value, "mitm"], payload,
  );
}
async function run(fn: () => Promise<void>) {
  busy.value = true;
  errorMessage.value = "";
  message.value = "";
  try {
    await fn();
  } catch (e) {
    errorMessage.value = e instanceof Error ? e.message : String(e);
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
    if (payload === persisted) {
      saveError.value = "";
      return;
    }
    const saved = await applySettings(payload);
    revision.value = saved.revision;
    persisted = payload;
    message.value = "";
    saveError.value = "";
  });
  return queue;
}
function saveChanges() {
  void flush().catch((error: unknown) => {
    message.value = "";
    saveError.value = "未保存：" + (error instanceof SyntaxError
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
  const data = await readSettings();
  const s = JSON.parse(data.content);
  const mitm: Site = {
    enabled: s.enabled,
    target_ip: s.target_ip || "192.2.0.1",
    certificate: s.certificate,
    private_key: s.private_key,
    routes: s.routes ?? [],
  };
  const path = await saveMitmBackup(
    JSON.stringify({ format: "netproxy-mitm", version: 1, mitm }, null, 2),
  );
  message.value = `备份已导出到 ${path}`;
}
async function importFile(e: Event) {
  const el = e.target as HTMLInputElement;
  const f = el.files?.[0];
  el.value = "";
  if (!f) return;
  await run(async () => {
    if (f.size > 262144) throw Error("备份超过 256 KiB");
    const text = await f.text();
    clearTimeout(timer);
    queue = queue.catch(() => {}).then(async () => {
      const result = importSite(text, draft());
      const saved = await applySettings(JSON.stringify(result.site));
      // A queued autosave must never write the pre-import form back to disk.
      fill(result.site);
      revision.value = saved.revision;
      persisted = JSON.stringify(draft());
      const current = await readSettings();
      showSaved(current);
      if (current.revision !== saved.revision) {
        throw Error("导入后配置已被其他操作修改，已显示当前保存的设置");
      }
      saveError.value = "";
      message.value = result.summary;
    });
    await queue;
  });
}
onMounted(() => run(read));
</script>
<template>
  <section>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
    <p v-if="saveError" class="form-error" role="alert">{{ saveError }}</p>

    <fieldset :disabled="busy || !revision">
      <label>启用 MITM <VanSwitch v-model="site.enabled" /></label>
      <VanField v-model="site.target_ip" label="目标 IPv4" />
      <VanField
        v-model="site.certificate"
        type="textarea"
        label-align="top"
        label="证书链 PEM"
        autosize
      />
      <VanField
        v-model="site.private_key"
        type="textarea"
        label-align="top"
        label="私钥 PEM"
        autosize
      />
      <h3>HTTPS 自定义接口</h3>
      <div v-for="(row, i) in rows" :key="i">
        <VanField v-model="row.path" label="路径" /><VanField
          v-model="row.body"
          label="响应 JSON"
          type="textarea"
          label-align="top"
          autosize
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
      <VanButton @click="run(backup)">导出备份</VanButton>
    </fieldset>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
