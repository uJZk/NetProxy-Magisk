import assert from "node:assert/strict"
import test from "node:test"

import { validateRelease } from "./release-preflight.mjs"

const changelog = `# 更新日志

## 版本 8.2.3

本次更新。

## 版本 8.2.2

上一版更新。
`

test("发布预检接受匹配的 tag、模块版本和更新日志", () => {
  const result = validateRelease({
    tag: "v8.2.3",
    moduleProp: "id=netproxy\nversion=v8.2.3\n",
    changelog,
  })

  assert.equal(result.version, "8.2.3")
  assert.match(result.notes, /本次更新/)
  assert.doesNotMatch(result.notes, /上一版更新/)
})

test("点分 beta 版本可以发布且必须匹配独立日志章节", () => {
  const result = validateRelease({ tag: "v8.6.3.beta1", moduleProp: "version=v8.6.3.beta1\n", changelog: "## 版本 8.6.3.beta1\n\n测试版更新。\n\n## 版本 8.6.3\n\n旧版。\n" })
  assert.match(result.notes, /测试版更新/)
  assert.doesNotMatch(result.notes, /旧版/)
  for (const tag of ["v8.6.3.beta", "v8.6.3.beta1.extra", "v8.6.3.foo1"]) {
    assert.throws(() => validateRelease({ tag, moduleProp: `version=${tag}\n`, changelog: "" }))
  }
})

test("发布预检拒绝不带 v 前缀的 tag", () => {
  assert.throws(() => validateRelease({
    tag: "8.2.3",
    moduleProp: "version=v8.2.3\n",
    changelog,
  }), /发布 tag 必须为/)
})

test("发布预检拒绝模块版本和 tag 不一致", () => {
  assert.throws(() => validateRelease({
    tag: "v8.2.3",
    moduleProp: "version=v8.2.2\n",
    changelog,
  }), /不一致/)
})

test("发布预检拒绝缺少对应更新日志的 tag", () => {
  assert.throws(() => validateRelease({
    tag: "v8.2.4",
    moduleProp: "version=v8.2.4\n",
    changelog,
  }), /找不到版本/)
})

test("发布预检拒绝只有标题或空白正文的更新日志", () => {
  for (const body of ["", "\n \t\n"]) {
    assert.throws(() => validateRelease({
      tag: "v8.2.3",
      moduleProp: "version=v8.2.3\n",
      changelog: `## 版本 8.2.3\n${body}\n## 版本 8.2.2\n旧版说明`,
    }), /更新日志为空/)
  }
})
