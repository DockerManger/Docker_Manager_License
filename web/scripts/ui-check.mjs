import puppeteer from 'puppeteer-core'
import { mkdirSync } from 'node:fs'

const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const BASE = 'http://localhost:5174'
mkdirSync('shots', { recursive: true })

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
})
const page = await browser.newPage()
await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 })

const errors = []
page.on('console', (m) => {
  if (m.type() === 'error') errors.push('[console] ' + m.text().slice(0, 180))
})
page.on('pageerror', (e) => errors.push('[pageerror] ' + String(e).slice(0, 180)))

async function login() {
  await page.goto(BASE, { waitUntil: 'load' })
  await sleep(1800)
  await page.screenshot({ path: 'shots/01-login-dark.png' })
  await page.evaluate(() => {
    const setVal = (el, v) => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
      setter.call(el, v)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    }
    const inputs = [...document.querySelectorAll('form input')]
    if (inputs.length >= 2) { setVal(inputs[0], 'admin'); setVal(inputs[1], 'admin123') }
  })
  await sleep(300)
  await page.evaluate(() => {
    document.querySelector('form')?.requestSubmit()
  })
  await sleep(2200)
}

await login()
console.log('login url:', page.url())
await page.screenshot({ path: 'shots/02-dashboard-dark.png' })
const dash = await page.evaluate(() => ({
  cards: document.querySelectorAll('[data-slot="card"]').length,
  tables: document.querySelectorAll('table').length,
  theme: document.documentElement.dataset.theme,
  brand: getComputedStyle(document.documentElement).getPropertyValue('--brand').trim(),
}))
console.log('dashboard:', JSON.stringify(dash))

// 各页面
for (const [name, path] of [
  ['licenses', '/licenses'],
  ['license-detail', '/licenses/DMG-01J2K3M4N5P6Q7R8S9T0U1V4'],
  ['customers', '/customers'],
  ['subscriptions', '/subscriptions'],
  ['security', '/security'],
  ['audit', '/audit'],
  ['settings', '/settings'],
]) {
  await page.goto(BASE + path, { waitUntil: 'load' })
  await sleep(1500)
  await page.screenshot({ path: `shots/03-${name}.png` })
  const info = await page.evaluate(() => ({
    rows: document.querySelectorAll('tbody tr').length,
    btns: document.querySelectorAll('[data-slot="button"]').length,
    badges: document.querySelectorAll('[data-slot="badge"]').length,
  }))
  console.log(name + ':', JSON.stringify(info))
}

// 签发 Dialog
await page.goto(BASE + '/licenses', { waitUntil: 'load' })
await sleep(1400)
const createBtn = await page.evaluate(() => {
  const el = [...document.querySelectorAll('[data-slot="button"]')].find((b) => b.textContent.includes('签发 License'))
  if (!el) return null
  const r = el.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (createBtn) {
  await page.mouse.click(createBtn.x, createBtn.y)
  await sleep(900)
}
const dlg = await page.evaluate(() => {
  const d = document.querySelector('[data-slot="dialog-content"]')
  if (!d) return { open: false }
  return {
    open: true,
    title: d.querySelector('[data-slot="dialog-title"]')?.textContent?.trim(),
    inputs: d.querySelectorAll('input').length,
    selects: d.querySelectorAll('[data-slot="select-trigger"]').length,
    checkboxes: d.querySelectorAll('[data-slot="checkbox"]').length,
  }
})
console.log('issue dialog:', JSON.stringify(dlg))
await page.screenshot({ path: 'shots/04-issue-dialog.png' })
await page.keyboard.press('Escape')
await sleep(500)

// 吊销 AlertDialog(点第一行的 ⋯ 菜单)
const menuPos = await page.evaluate(() => {
  const btn = document.querySelector('[data-slot="button"][data-size="sm"]')
  if (!btn) return null
  const r = btn.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (menuPos) {
  await page.mouse.click(menuPos.x, menuPos.y)
  await sleep(700)
  await page.screenshot({ path: 'shots/05-dropdown-menu.png' })
  const menuItems = await page.evaluate(() =>
    [...document.querySelectorAll('[data-slot="dropdown-menu-content"] [data-slot="dropdown-menu-item"]')].map((i) => i.textContent.trim()),
  )
  console.log('dropdown items:', JSON.stringify(menuItems))
  await page.keyboard.press('Escape')
  await sleep(400)
}

// Select 下拉
await page.evaluate(() => {
  document.querySelector('[data-slot="select-trigger"]')?.click()
})
await sleep(700)
const sel = await page.evaluate(() => {
  const items = [...document.querySelectorAll('[data-slot="select-content"] [data-slot="select-item"]')]
  return { count: items.length, first: items[0]?.textContent?.trim() }
})
console.log('select:', JSON.stringify(sel))
await page.keyboard.press('Escape')
await sleep(400)

// 亮色主题
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => b.getAttribute('aria-label')?.includes('亮色') || b.getAttribute('aria-label')?.includes('暗色'))
  btn?.click()
})
await sleep(1300)
console.log('theme after toggle:', await page.evaluate(() => document.documentElement.dataset.theme))
await page.screenshot({ path: 'shots/06-dashboard-light.png' })
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => b.getAttribute('aria-label')?.includes('亮色') || b.getAttribute('aria-label')?.includes('暗色'))
  btn?.click()
})
await sleep(800)

// 移动端(Sheet 导航)
await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 1 })
await page.goto(BASE + '/licenses', { waitUntil: 'load' })
await sleep(1500)
const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
console.log('mobile overflow:', overflow)
await page.screenshot({ path: 'shots/07-mobile-licenses.png' })
// 打开移动菜单
const menuBtn = await page.evaluate(() => {
  const el = [...document.querySelectorAll('button')].find((b) => b.querySelector('svg') && !b.querySelector('svg path[d*="M3 6h18"]'))
  const menu = [...document.querySelectorAll('button')].find((b) => {
    const svg = b.querySelector('svg')
    return svg && b.closest('.lg\\:hidden')
  })
  return null
})
// 用 aria 或位置:移动菜单是 header 第一个按钮
const mPos = await page.evaluate(() => {
  const header = document.querySelector('header')
  const btn = header?.querySelector('button')
  if (!btn) return null
  const r = btn.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (mPos) {
  await page.mouse.click(mPos.x, mPos.y)
  await sleep(800)
}
const sheet = await page.evaluate(() => {
  const s = document.querySelector('[data-slot="sheet-content"]')
  if (!s) return { open: false }
  return { open: true, links: s.querySelectorAll('a').length }
})
console.log('mobile sheet:', JSON.stringify(sheet))
await page.screenshot({ path: 'shots/08-mobile-sheet.png' })

console.log('\n=== errors ===')
console.log(errors.length ? errors.slice(0, 10).join('\n') : 'none')
await browser.close()
