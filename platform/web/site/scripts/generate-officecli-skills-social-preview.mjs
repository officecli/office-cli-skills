import fs from 'node:fs'
import path from 'node:path'
import zlib from 'node:zlib'

const WIDTH = 1280
const HEIGHT = 640

const colors = {
  background: hex('#131313'),
  surface: hex('#17191F'),
  surfaceLow: hex('#10161C'),
  surfaceAlt: hex('#0F141A'),
  stroke: hex('#2A3140'),
  primary: hex('#AEC6FF'),
  primaryStrong: hex('#4F8EFF'),
  tertiary: hex('#00DCE5'),
  secondary: hex('#FFB1C0'),
  white: hex('#FFFFFF'),
  muted: hex('#8B90A0'),
  mutedDark: hex('#6C7387'),
}

const font = {
  A: ['01110', '10001', '10001', '11111', '10001', '10001', '10001'],
  B: ['11110', '10001', '10001', '11110', '10001', '10001', '11110'],
  C: ['01111', '10000', '10000', '10000', '10000', '10000', '01111'],
  D: ['11110', '10001', '10001', '10001', '10001', '10001', '11110'],
  E: ['11111', '10000', '10000', '11110', '10000', '10000', '11111'],
  F: ['11111', '10000', '10000', '11110', '10000', '10000', '10000'],
  G: ['01111', '10000', '10000', '10111', '10001', '10001', '01111'],
  H: ['10001', '10001', '10001', '11111', '10001', '10001', '10001'],
  I: ['11111', '00100', '00100', '00100', '00100', '00100', '11111'],
  J: ['00111', '00010', '00010', '00010', '00010', '10010', '01100'],
  K: ['10001', '10010', '10100', '11000', '10100', '10010', '10001'],
  L: ['10000', '10000', '10000', '10000', '10000', '10000', '11111'],
  M: ['10001', '11011', '10101', '10101', '10001', '10001', '10001'],
  N: ['10001', '10001', '11001', '10101', '10011', '10001', '10001'],
  O: ['01110', '10001', '10001', '10001', '10001', '10001', '01110'],
  P: ['11110', '10001', '10001', '11110', '10000', '10000', '10000'],
  Q: ['01110', '10001', '10001', '10001', '10101', '10010', '01101'],
  R: ['11110', '10001', '10001', '11110', '10100', '10010', '10001'],
  S: ['01111', '10000', '10000', '01110', '00001', '00001', '11110'],
  T: ['11111', '00100', '00100', '00100', '00100', '00100', '00100'],
  U: ['10001', '10001', '10001', '10001', '10001', '10001', '01110'],
  V: ['10001', '10001', '10001', '10001', '10001', '01010', '00100'],
  W: ['10001', '10001', '10001', '10101', '10101', '10101', '01010'],
  X: ['10001', '10001', '01010', '00100', '01010', '10001', '10001'],
  Y: ['10001', '10001', '01010', '00100', '00100', '00100', '00100'],
  Z: ['11111', '00001', '00010', '00100', '01000', '10000', '11111'],
  '0': ['01110', '10001', '10011', '10101', '11001', '10001', '01110'],
  '1': ['00100', '01100', '00100', '00100', '00100', '00100', '01110'],
  '2': ['01110', '10001', '00001', '00010', '00100', '01000', '11111'],
  '3': ['11110', '00001', '00001', '01110', '00001', '00001', '11110'],
  '4': ['00010', '00110', '01010', '10010', '11111', '00010', '00010'],
  '5': ['11111', '10000', '10000', '11110', '00001', '00001', '11110'],
  '6': ['01111', '10000', '10000', '11110', '10001', '10001', '01110'],
  '7': ['11111', '00001', '00010', '00100', '01000', '01000', '01000'],
  '8': ['01110', '10001', '10001', '01110', '10001', '10001', '01110'],
  '9': ['01110', '10001', '10001', '01111', '00001', '00001', '11110'],
  '-': ['00000', '00000', '00000', '11111', '00000', '00000', '00000'],
  '/': ['00001', '00010', '00100', '01000', '10000', '00000', '00000'],
  '.': ['00000', '00000', '00000', '00000', '00000', '01100', '01100'],
  ':': ['00000', '01100', '01100', '00000', '01100', '01100', '00000'],
  '$': ['00100', '01111', '10100', '01110', '00101', '11110', '00100'],
  ' ': ['00000', '00000', '00000', '00000', '00000', '00000', '00000'],
}

const canvas = Buffer.alloc(WIDTH * HEIGHT * 4, 0)

for (let y = 0; y < HEIGHT; y += 1) {
  for (let x = 0; x < WIDTH; x += 1) {
    setPixel(x, y, colors.background)
  }
}

radialGlow(982, 110, 250, { ...colors.tertiary, a: 38 })
radialGlow(1030, 218, 174, { ...colors.primaryStrong, a: 32 })
radialGlow(260, 524, 236, { ...colors.primaryStrong, a: 28 })

strokeRoundedRect(60, 56, 1160, 528, 36, { ...colors.stroke, a: 255 }, colors.surface)
fillRoundedRect(104, 92, 244, 40, 20, colors.surfaceLow)
drawText('NEW BRAND SYSTEM', 128, 102, 3, colors.tertiary)

drawOfficeCliMark(112, 150, 332)

drawText('OFFICECLI', 536, 148, 10, colors.white, 2)
drawText('SKILLS', 536, 242, 10, colors.primary, 2)
drawText('CLAUDE CODE  CODEX  AI AGENTS', 538, 348, 3, colors.primary)
drawText('PPTX  DOCX  XLSX  REPORT', 538, 392, 4, colors.white)
drawText('SHARE PREVIEW UPDATED WITH THE NEW LOGO', 538, 436, 2, colors.muted)

fillRoundedRect(536, 486, 424, 44, 22, colors.surfaceLow)
drawText('GITHUB REPO  OFFICECLI/OFFICECLI-SKILLS', 564, 498, 2, colors.muted)

fillRoundedRect(972, 486, 176, 44, 22, colors.surfaceAlt)
drawText('SOCIAL PREVIEW', 1002, 498, 2, colors.secondary)

const outputArg = process.argv[2]
const outputPath = outputArg
  ? path.resolve(outputArg)
  : path.resolve(import.meta.dirname, '../public/social-preview-officecli-skills.png')

fs.mkdirSync(path.dirname(outputPath), { recursive: true })
fs.writeFileSync(outputPath, encodePNG(WIDTH, HEIGHT, canvas))
console.log(outputPath)

function hex(value) {
  const normalized = value.replace('#', '')
  return {
    r: Number.parseInt(normalized.slice(0, 2), 16),
    g: Number.parseInt(normalized.slice(2, 4), 16),
    b: Number.parseInt(normalized.slice(4, 6), 16),
    a: 255,
  }
}

function setPixel(x, y, color) {
  if (x < 0 || y < 0 || x >= WIDTH || y >= HEIGHT) return
  const index = (y * WIDTH + x) * 4
  const alpha = (color.a ?? 255) / 255
  const inv = 1 - alpha
  canvas[index] = Math.round((color.r ?? 0) * alpha + canvas[index] * inv)
  canvas[index + 1] = Math.round((color.g ?? 0) * alpha + canvas[index + 1] * inv)
  canvas[index + 2] = Math.round((color.b ?? 0) * alpha + canvas[index + 2] * inv)
  canvas[index + 3] = 255
}

function fillRect(x, y, width, height, color) {
  const x0 = Math.max(0, Math.floor(x))
  const y0 = Math.max(0, Math.floor(y))
  const x1 = Math.min(WIDTH, Math.ceil(x + width))
  const y1 = Math.min(HEIGHT, Math.ceil(y + height))
  for (let py = y0; py < y1; py += 1) {
    for (let px = x0; px < x1; px += 1) {
      setPixel(px, py, color)
    }
  }
}

function fillCircle(cx, cy, radius, color) {
  const r2 = radius * radius
  const x0 = Math.floor(cx - radius)
  const x1 = Math.ceil(cx + radius)
  const y0 = Math.floor(cy - radius)
  const y1 = Math.ceil(cy + radius)
  for (let y = y0; y <= y1; y += 1) {
    for (let x = x0; x <= x1; x += 1) {
      const dx = x - cx
      const dy = y - cy
      if (dx * dx + dy * dy <= r2) setPixel(x, y, color)
    }
  }
}

function fillRoundedRect(x, y, width, height, radius, color) {
  fillRect(x + radius, y, width - radius * 2, height, color)
  fillRect(x, y + radius, radius, height - radius * 2, color)
  fillRect(x + width - radius, y + radius, radius, height - radius * 2, color)
  fillCircle(x + radius, y + radius, radius, color)
  fillCircle(x + width - radius, y + radius, radius, color)
  fillCircle(x + radius, y + height - radius, radius, color)
  fillCircle(x + width - radius, y + height - radius, radius, color)
}

function strokeRoundedRect(x, y, width, height, radius, color, fillColor = null) {
  fillRoundedRect(x, y, width, height, radius, color)
  if (fillColor) {
    fillRoundedRect(x + 2, y + 2, width - 4, height - 4, Math.max(0, radius - 2), fillColor)
  }
}

function radialGlow(cx, cy, radius, color) {
  const x0 = Math.floor(cx - radius)
  const x1 = Math.ceil(cx + radius)
  const y0 = Math.floor(cy - radius)
  const y1 = Math.ceil(cy + radius)
  for (let y = y0; y <= y1; y += 1) {
    for (let x = x0; x <= x1; x += 1) {
      const dx = x - cx
      const dy = y - cy
      const distance = Math.sqrt(dx * dx + dy * dy)
      if (distance > radius) continue
      const strength = 1 - distance / radius
      const alpha = Math.round((color.a ?? 255) * strength * strength)
      setPixel(x, y, { ...color, a: alpha })
    }
  }
}

function drawLine(x1, y1, x2, y2, thickness, color) {
  const dx = x2 - x1
  const dy = y2 - y1
  const steps = Math.max(Math.abs(dx), Math.abs(dy))
  for (let i = 0; i <= steps; i += 1) {
    const x = x1 + (dx * i) / steps
    const y = y1 + (dy * i) / steps
    fillCircle(x, y, thickness / 2, color)
  }
}

function drawText(text, x, y, scale, color, letterSpacing = 1) {
  let cursor = x
  for (const rawChar of text) {
    const char = rawChar.toUpperCase()
    const glyph = font[char] ?? font[' ']
    for (let row = 0; row < glyph.length; row += 1) {
      for (let col = 0; col < glyph[row].length; col += 1) {
        if (glyph[row][col] !== '1') continue
        fillRect(cursor + col * scale, y + row * scale, scale, scale, color)
      }
    }
    cursor += glyph[0].length * scale + letterSpacing * scale
  }
}

function drawOfficeCliMark(x, y, size) {
  fillRoundedRect(x, y, size, size, 68, colors.surfaceLow)
  strokeRoundedRect(x, y, size, size, 68, { ...colors.stroke, a: 255 }, { ...colors.surfaceLow, a: 255 })

  radialGlow(x + size * 0.73, y + size * 0.19, size * 0.18, { ...colors.tertiary, a: 54 })
  radialGlow(x + size * 0.38, y + size * 0.73, size * 0.24, { ...colors.primaryStrong, a: 40 })

  const frame = { ...colors.primary, a: 244 }
  drawLine(x + 66, y + 135, x + 190, y + 88, 20, frame)
  drawLine(x + 190, y + 88, x + 238, y + 134, 20, frame)
  drawLine(x + 66, y + 135, x + 66, y + 256, 20, frame)
  drawLine(x + 66, y + 256, x + 136, y + 256, 20, frame)

  fillRoundedRect(x + 188, y + 64, 94, 198, 22, { ...colors.tertiary, a: 214 })
  strokeRoundedRect(x + 188, y + 64, 94, 198, 22, { ...colors.tertiary, a: 255 }, { r: 28, g: 162, b: 181, a: 118 })
  drawLine(x + 236, y + 64, x + 236, y + 96, 8, colors.white)
  drawLine(x + 236, y + 96, x + 264, y + 96, 8, colors.white)

  strokeRoundedRect(x + 92, y + 126, 92, 76, 12, { ...colors.stroke, a: 255 }, { r: 8, g: 15, b: 22, a: 255 })
  drawLine(x + 118, y + 148, x + 140, y + 164, 10, colors.white)
  drawLine(x + 140, y + 164, x + 118, y + 180, 10, colors.white)
  drawLine(x + 154, y + 180, x + 178, y + 180, 10, colors.white)
}

function encodePNG(width, height, rgba) {
  const stride = width * 4
  const raw = Buffer.alloc((stride + 1) * height)
  for (let y = 0; y < height; y += 1) {
    raw[y * (stride + 1)] = 0
    rgba.copy(raw, y * (stride + 1) + 1, y * stride, (y + 1) * stride)
  }

  const header = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(width, 0)
  ihdr.writeUInt32BE(height, 4)
  ihdr[8] = 8
  ihdr[9] = 6
  ihdr[10] = 0
  ihdr[11] = 0
  ihdr[12] = 0

  const idat = zlib.deflateSync(raw, { level: 9 })
  return Buffer.concat([header, chunk('IHDR', ihdr), chunk('IDAT', idat), chunk('IEND', Buffer.alloc(0))])
}

function chunk(type, data) {
  const name = Buffer.from(type)
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length, 0)
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(Buffer.concat([name, data])), 0)
  return Buffer.concat([length, name, data, crc])
}

function crc32(data) {
  let crc = 0xffffffff
  for (let i = 0; i < data.length; i += 1) {
    crc ^= data[i]
    for (let bit = 0; bit < 8; bit += 1) {
      const mask = -(crc & 1)
      crc = (crc >>> 1) ^ (0xedb88320 & mask)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}
