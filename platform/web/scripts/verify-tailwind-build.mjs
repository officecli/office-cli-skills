import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'

const appName = process.argv[2]

if (!appName || !['app', 'admin'].includes(appName)) {
  console.error('usage: node ../scripts/verify-tailwind-build.mjs <app|admin>')
  process.exit(1)
}

const packageDir = process.cwd()
const distAssetsDir = path.join(packageDir, 'dist', 'assets')

const requiredSelectors = [
  '.min-h-screen',
  '.px-6',
  '.text-white',
  '.bg-background',
]

function readCssBundleText(dir) {
  const entries = readdirSync(dir)
    .filter((name) => name.endsWith('.css'))
    .map((name) => path.join(dir, name))
    .filter((file) => statSync(file).isFile())

  if (entries.length === 0) {
    throw new Error(`no css assets found in ${dir}`)
  }

  return entries.map((file) => readFileSync(file, 'utf8')).join('\n')
}

const cssText = readCssBundleText(distAssetsDir)
const missingSelectors = requiredSelectors.filter((selector) => !cssText.includes(selector))

if (missingSelectors.length > 0) {
  console.error(`[${appName}] missing compiled Tailwind selectors:`)
  for (const selector of missingSelectors) {
    console.error(`- ${selector}`)
  }
  process.exit(1)
}

console.log(`[${appName}] verified compiled Tailwind selectors: ${requiredSelectors.join(', ')}`)
