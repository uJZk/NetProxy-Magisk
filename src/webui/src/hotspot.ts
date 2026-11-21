import './hotspot.css'
import { ctlJson, inKsu, shell } from './exec'
import { decodeCtlResult } from './contract'

type DNS = { domain: string; address: string }
type Route = { path: string; json: unknown }
type Settings = { enabled: boolean; domain: string; certificate: string; private_key: string; dns_overrides: DNS[] | null; dns_block: string[] | null; dns_allow: string[] | null; routes: Route[] | null }
type Config = { content: string; revision: string; status: string }
const el = <T extends HTMLElement>(id: string) => document.getElementById(id) as T
const message = el<HTMLDivElement>('message'), form = el<HTMLFormElement>('settings')
const enabled = el<HTMLInputElement>('enabled'), domain = el<HTMLInputElement>('domain')
const cert = el<HTMLTextAreaElement>('cert'), key = el<HTMLTextAreaElement>('key')
const dns = el<HTMLDivElement>('dns'), routes = el<HTMLDivElement>('routes'), save = el<HTMLButtonElement>('save')
const dnsBlock = el<HTMLTextAreaElement>('dns-block'), dnsAllow = el<HTMLTextAreaElement>('dns-allow')
let revision = ''
let loaded = false
const say = (s: string) => { message.textContent = s }
function field(labelText: string, value: string, multiline = false): HTMLInputElement | HTMLTextAreaElement {
  const label = document.createElement('label'); label.textContent = labelText
  const input = document.createElement(multiline ? 'textarea' : 'input') as HTMLInputElement | HTMLTextAreaElement
  input.value = value; input.spellcheck = false; label.append(input); return input
}
function appendRow(container: HTMLElement, one: HTMLInputElement | HTMLTextAreaElement, two: HTMLInputElement | HTMLTextAreaElement) {
  const row = document.createElement('div'); row.className = 'row'; row.append(one.parentElement!, two.parentElement!)
  const remove = document.createElement('button'); remove.type = 'button'; remove.textContent = '删除'; remove.addEventListener('click', () => row.remove()); row.append(remove); container.append(row)
}
function addDNS(d: DNS = { domain: '', address: '' }) { appendRow(dns, field('域名', d.domain), field('IPv4 地址（可留空）', d.address)) }
function addRoute(r: Route = { path: '', json: {} }) { appendRow(routes, field('路径', r.path), field('JSON 响应', JSON.stringify(r.json, null, 2), true)) }
function rows(container: HTMLElement): Array<[string, string]> { return [...container.querySelectorAll<HTMLDivElement>('.row')].map(row => { const inputs = row.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('input,textarea'); return [inputs[0].value, inputs[1].value] }) }
function fill(s: Settings) {
  enabled.checked = s.enabled; domain.value = s.domain; cert.value = s.certificate; key.value = ''
  dnsBlock.value = (s.dns_block || []).join('\n'); dnsAllow.value = (s.dns_allow || []).join('\n')
  dns.replaceChildren(); routes.replaceChildren(); (s.dns_overrides || []).forEach(addDNS); (s.routes || []).forEach(addRoute)
}
async function read() {
  loaded = false; save.disabled = true
  say('正在读取…'); const result = await ctlJson<Config>(['config', 'read', 'hotspot'])
  if (!result.ok || !result.data) { say(result.message || '读取失败'); return }
  try { const s = JSON.parse(result.data.content) as Settings; const status = JSON.parse(result.data.status) as { state: string; message: string }; revision = result.data.revision; fill(s); loaded = true; save.disabled = false; say(`站点状态：${status.message || status.state}`) } catch { say('无法解析站点配置或状态。') }
}
function quote(s: string) { return "'" + s.replace(/'/g, "'\\''") + "'" }
function base64(s: string) { const bytes = new TextEncoder().encode(s); let text = ''; for (const b of bytes) text += String.fromCharCode(b); return btoa(text) }
async function persist(s: Settings) {
  const payload = base64(JSON.stringify(s)); const path = '/data/adb/modules/netproxy/netproxyctl'
  const cmd = `umask 077; task_hotspot_file=$(mktemp /data/local/tmp/netproxy-site.XXXXXXXX) || exit 1; trap 'rm -f "$task_hotspot_file"' EXIT; printf %s ${quote(payload)} | base64 -d > "$task_hotspot_file" || exit 1; ${quote(path)} --json --timeout 180s config apply --revision ${quote(revision)} hotspot "$task_hotspot_file"; task_hotspot_result=$?; exit "$task_hotspot_result"`
  return decodeCtlResult<unknown>(await shell(cmd))
}
el<HTMLButtonElement>('add-dns').addEventListener('click', () => addDNS())
el<HTMLButtonElement>('add-route').addEventListener('click', () => addRoute())
el<HTMLButtonElement>('reload').addEventListener('click', () => { void read() })
el<HTMLButtonElement>('export').addEventListener('click', async () => {
  const result = await ctlJson<Config>(['config', 'read', '--include-private', 'hotspot'])
  if (!result.ok || !result.data) { say(result.message || '导出失败'); return }
  const object = URL.createObjectURL(new Blob([result.data.content], { type: 'application/json' }))
  const a = document.createElement('a'); a.href = object; a.download = 'netproxy-hotspot.json'; document.body.append(a); a.click(); a.remove()
  window.setTimeout(() => URL.revokeObjectURL(object), 5000); say('备份已导出，文件可能包含私钥。')
})
el<HTMLInputElement>('import').addEventListener('change', async event => {
  const input = event.currentTarget as HTMLInputElement; const file = input.files?.[0]; if (!file) return
  try {
    if (file.size > 256 * 1024) throw new Error('备份文件过大')
    const s = JSON.parse(await file.text()) as Settings
    if (!s || typeof s.domain !== 'string' || !Array.isArray(s.routes) || (s.dns_overrides !== null && !Array.isArray(s.dns_overrides))) throw new Error('备份格式无效')
    fill(s); key.value = s.private_key || ''; say('备份已载入，点击保存后生效。')
  } catch (e) { say(e instanceof Error ? e.message : '导入失败') } finally { input.value = '' }
})
form.addEventListener('submit', async event => {
  event.preventDefault(); save.disabled = true
  try {
    if (!inKsu) throw new Error('请在模块管理器中打开此页面。')
    const lines = (value: string) => value.split('\n').map(s => s.trim()).filter(s => s && !s.startsWith('#'))
    const s: Settings = { enabled: enabled.checked, domain: domain.value.trim(), certificate: cert.value.trim(), private_key: key.value.trim(), dns_block: lines(dnsBlock.value), dns_allow: lines(dnsAllow.value), dns_overrides: rows(dns).map(([domain, address]) => ({ domain: domain.trim(), address: address.trim() })), routes: rows(routes).map(([path, json]) => ({ path: path.trim(), json: JSON.parse(json) as unknown })) }
    say('正在保存并应用…'); const result = await persist(s)
    if (!result.ok) { say(result.message); return }
    await read(); say('设置已保存；后台状态可能需要数秒更新，请点击重新读取。')
  } catch (e) { say(e instanceof Error ? e.message : String(e)) } finally { save.disabled = !loaded }
})
void read()
