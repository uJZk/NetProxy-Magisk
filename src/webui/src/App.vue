<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from "vue";
import {
  Cell,
  CellGroup,
  Field as VanField,
  Button as VanButton,
  Switch as VanSwitch,
} from "vant";
import { Alert as VarAlert } from "@varlet/ui";
import { JSONUIProvider, Renderer } from "@json-render/vue";
import { command, submitFile, type Config } from "./control";
import Mitm from "./Mitm.vue";
const current = ref(location.hash === "#mitm" ? "MITM" : "");
const panel = ref(""),
  rawURL = ref(""),
  interval = ref(0),
  auto = ref(false);
const running = ref(false),
  serviceLoaded = ref(false);
const busy = ref(false),
  message = ref(""),
  autoRevision = ref(""),
  rawLoaded = ref(false);
const logKind = ref("core"),
  logText = ref("");
const channel = ref("stable"),
  versions = ref<Record<string, string>>({});
async function refreshCores() {
  versions.value = await command<Record<string, string>>("core", "status");
}
const names = ["sing-box 配置", "sing-box 面板", "MITM", "日志"];
async function run(fn: () => Promise<void>) {
  busy.value = true;
  message.value = "";
  try {
    await fn();
  } catch (e) {
    message.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  const status = await command<{ pid?: number }>("service", "status");
  running.value = Boolean(status.pid);
  serviceLoaded.value = true;
  const raw = await command<{
    url: string;
    interval: number;
  }>("config", "raw", "show");
  rawURL.value = raw.url;
  interval.value = raw.interval;
  rawLoaded.value = true;
  const conf = await command<Config>("config", "read", "module/auto_start");
  autoRevision.value = conf.revision;
  auto.value = JSON.parse(conf.content).auto_start;
  panel.value = (await command<{ url: string }>("config", "raw", "panel")).url;
}
async function logs() {
  const data = await command<{ content: string }>(
    "logs",
    "show",
    logKind.value,
  );
  logText.value = data.content || "暂无日志";
}
function open(name: string) {
  message.value = "";
  if (name === "sing-box 面板") {
    if (panel.value) location.assign(panel.value);
    return;
  }
  current.value = name;
  if (name === "日志") void run(logs);
  if (name === "sing-box 配置") void run(refreshCores);
}
const registry = {
  Menu: defineComponent({
    setup: () => () =>
      h(CellGroup, { inset: true }, () =>
        names.map((name) => {
          const disabled = name === "sing-box 面板" && !panel.value;
          return h(Cell, {
            title: name,
            isLink: !disabled,
            clickable: !disabled,
            label: disabled ? "原始配置未提供可用面板地址" : undefined,
            role: "button",
            tabindex: disabled ? -1 : 0,
            "aria-disabled": disabled,
            class: { muted: disabled },
            onClick: () => !disabled && open(name),
            onKeydown: (e: KeyboardEvent) => {
              if (!disabled && e.key === "Enter") open(name);
            },
          });
        }),
      ),
  }),
};
const spec = {
  root: "navigation",
  elements: { navigation: { type: "Menu", props: {}, children: [] } },
};
const title = computed(() => current.value || "NetProxy");
async function saveAuto(value: boolean) {
  await run(async () => {
    await submitFile(
      [
        "config",
        "apply",
        "--revision",
        autoRevision.value,
        "module/auto_start",
      ],
      JSON.stringify({ auto_start: value }),
    );
    await refresh();
    message.value = "开机设置已保存";
  });
}
async function update() {
  await command(
    "config",
    "raw",
    "set",
    "--interval",
    String(interval.value),
    rawURL.value,
  );
  await command("config", "raw", "update", "--force");
  await command("config", "raw", "enable");
  await refresh();
  message.value = "配置已更新";
}
async function importFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  await run(async () => {
    await submitFile(["config", "raw", "import"], await file.text());
    await refresh();
    message.value = "文件已导入，已切换为本地来源";
  });
}
onMounted(() => run(refresh));
</script>
<template>
  <main>
    <header>
      <button
        v-if="current"
        @click="
          current = '';
          message = '';
          run(refresh);
        "
      >
        ‹ 返回
      </button>
      <h1>{{ title }}</h1>
    </header>
    <JSONUIProvider v-if="!current" :registry="registry"
      ><Renderer :spec="spec" :registry="registry"
    /></JSONUIProvider>
    <Mitm v-else-if="current === 'MITM'" />
    <section v-else-if="current === 'sing-box 配置'">
      <fieldset :disabled="busy || !rawLoaded">
        <label
          >sing-box 总开关
          <VanSwitch
            :model-value="running"
            :disabled="!serviceLoaded"
            @update:model-value="
              (value) =>
                run(async () => {
                  await command('service', value ? 'start' : 'stop');
                  await refresh();
                })
            "
        /></label>
        <label
          >开机自启
          <VanSwitch
            :model-value="auto"
            :disabled="!autoRevision"
            @update:model-value="saveAuto"
        /></label>
        <VanField
          v-model="rawURL"
          label="配置 URL"
          type="url"
          placeholder="https://"
        />
        <VanField
          v-model.number="interval"
          label="更新间隔（秒）"
          type="digit"
          placeholder="0 表示仅手动更新"
        />
        <VanButton type="primary" @click="run(update)">从 URL 更新</VanButton>
        <label
          >从文件导入<input
            type="file"
            accept=".json,.jsonc"
            @change="importFile"
        /></label>
        <VanButton
          @click="
            run(async () => {
              await command('config', 'raw', 'clear-cache');
              message = '下载缓存已清除';
            })
          "
          >清除下载缓存</VanButton
        >
        <h3>sing-box 核心</h3>
        <p>当前版本 {{ versions.active || "未知" }}</p>
        <label
          >渠道<select v-model="channel">
            <option value="stable">正式版</option>
            <option value="testing">测试版</option>
          </select></label
        >
        <p>已下载 {{ versions[channel] || "尚未下载" }}</p>
        <VanButton
          @click="
            run(async () => {
              await command('core', 'update', channel);
              await refreshCores();
              message = '下载和校验完成';
            })
          "
          >更新核心</VanButton
        >
        <VanButton
          :disabled="
            !versions[channel] || channel === versions.active_channel
          "
          @click="
            run(async () => {
              await command('core', 'switch', channel);
              await refreshCores();
              message = '核心已切换';
            })
          "
          >切换核心</VanButton
        >
      </fieldset>
    </section>
    <section v-else-if="current === '日志'">
      <fieldset :disabled="busy">
        <label
          >模块<select v-model="logKind" @change="run(logs)">
            <option value="core">sing-box</option>
            <option value="service">模块服务</option>
            <option value="config">配置更新</option>
            <option value="mitm">MITM / HTTPS</option>
          </select></label
        >
        <VanButton @click="run(logs)">刷新日志</VanButton>
      </fieldset>
      <pre>{{ logText }}</pre>
    </section>
    <VarAlert v-if="message" :message="message" role="status" />
  </main>
</template>
<style>
:root {
  font-family: Roboto, system-ui, sans-serif;
  color: #172d33;
  background: #eef3f2;
  --van-primary-color: #136658;
}
body {
  margin: 0;
}
main {
  max-width: 680px;
  margin: auto;
  padding: clamp(16px, 4vw, 32px);
}
header {
  margin: 1.5rem 0;
}
header button {
  border: 0;
  background: none;
  color: #136658;
  font-size: 1rem;
}
h1 {
  font-size: 2rem;
}
section {
  background: white;
  padding: 1rem;
  border-radius: 12px;
}
fieldset {
  border: 0;
  padding: 0;
  min-width: 0;
}
label {
  display: flex;
  gap: 0.7rem;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  flex-wrap: wrap;
}
button,
.van-button {
  margin: 4px;
}
input,
select {
  max-width: 100%;
}
.muted {
  opacity: 0.55;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 60vh;
  overflow: auto;
}
.van-cell-group--inset {
  margin: 0;
}
section h2 {
  margin: 0 0 10px;
}
</style>
