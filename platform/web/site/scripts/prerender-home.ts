import fs from 'node:fs'
import path from 'node:path'
import { JSDOM } from 'jsdom'
import { renderRouteApp, sitePrerenderRoutes } from '../src/prerender'
import { applyDocumentSEO, getRouteSEO } from '../src/seo'

const distDir = path.resolve(import.meta.dirname, '..', 'dist')
const indexPath = path.join(distDir, 'index.html')

if (!fs.existsSync(indexPath)) {
  throw new Error(`Missing built index.html at ${indexPath}`)
}

const html = fs.readFileSync(indexPath, 'utf8')

for (const route of sitePrerenderRoutes) {
  const dom = new JSDOM(html)
  const { document } = dom.window
  const root = document.getElementById('root')
  if (!root) {
    throw new Error(`Failed to locate #root for prerender route ${route}`)
  }

  root.innerHTML = renderRouteApp(route)
  applyDocumentSEO(document, getRouteSEO(route))

  const targetPath =
    route === '/' ? indexPath : path.join(distDir, route.replace(/^\//, ''), 'index.html')

  fs.mkdirSync(path.dirname(targetPath), { recursive: true })
  fs.writeFileSync(targetPath, dom.serialize())
}
