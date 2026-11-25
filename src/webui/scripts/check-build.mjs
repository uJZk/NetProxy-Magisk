import { readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export function checkBuild(directory) {
  for (const page of ['index.html','hotspot.html']) {
    const html=readFileSync(resolve(directory,page),'utf8')
    for(const match of html.matchAll(/\b(?:src|href)=["']([^"']+)["']/g)){
      const name=match[1].split(/[?#]/)[0]
      if(!name || /^(?:https?:|data:)/.test(name))continue
      const info=statSync(resolve(directory,name))
      if(!info.isFile()||!info.size)throw Error('WebUI 构建资源无效：'+name)
    }
  }
}
if(process.argv[1] && import.meta.url===pathToFileURL(process.argv[1]).href)checkBuild(resolve(import.meta.dirname,'../../module/webroot/netproxy'))
