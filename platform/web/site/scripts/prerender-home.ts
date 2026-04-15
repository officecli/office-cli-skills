import fs from 'node:fs'
import path from 'node:path'
import { renderRouteApp } from '../src/prerender'

const distDir = path.resolve(import.meta.dirname, '..', 'dist')
const indexPath = path.join(distDir, 'index.html')

if (!fs.existsSync(indexPath)) {
  throw new Error(`Missing built index.html at ${indexPath}`)
}

const html = fs.readFileSync(indexPath, 'utf8')
const appHTML = renderRouteApp('/')
const rendered = html.replace('<div id="root"></div>', `<div id="root">${appHTML}</div>`)

if (rendered === html) {
  throw new Error('Failed to inject prerendered app HTML into dist/index.html')
}

fs.writeFileSync(indexPath, rendered)
