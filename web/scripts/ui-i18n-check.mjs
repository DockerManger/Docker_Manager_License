import puppeteer from 'puppeteer-core'

const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const BASE = 'http://localhost:5174'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
})
const page = await browser.newPage()
await page.setViewport({ width: 1440, height: 900 })

const errors = []
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 160)) })
page.on('pageerror', (e) => errors.push(String(e).slice(0, 160)))

// 登录
await page.goto(BASE, { waitUntil: 'load' })
await sleep(1600)
await page.screenshot({ path: 'shots/11-login-new-logo.png' })
await page.evaluate(() => {
  const setVal = (el, v) => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
    setter.call(el, v)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }
  const inputs = [...document.querySelectorAll('form input')]
  if (inputs.length >= 2) { setVal(inputs[0], 'admin'); setVal(inputs[1], 'admin123') }
})
await sleep(200)
await page.evaluate(() => document.querySelector('form')?.requestSubmit())
await sleep(2200)

// 侧边栏收起状态
const collapsedW = await page.evaluate(() => document.querySelector('aside')?.getBoundingClientRect().width)
console.log('sidebar collapsed width:', collapsedW)
await page.screenshot({ path: 'shots/12-dashboard-collapsed.png' })

// hover 展开
const asidePos = await page.evaluate(() => {
  const r = document.querySelector('aside')?.getBoundingClientRect()
  return r ? { x: r.x + 20, y: r.y + 200 } : null
})
if (asidePos) {
  await page.mouse.move(asidePos.x, asidePos.y)
  await sleep(600)
}
const expandedW = await page.evaluate(() => document.querySelector('aside')?.getBoundingClientRect().width)
console.log('sidebar expanded width:', expandedW)
await page.screenshot({ path: 'shots/13-dashboard-expanded.png' })

// 移出收起
await page.mouse.move(900, 400)
await sleep(500)

// 语言切换 -> English(点 header 的语言按钮)
const langBtn = await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || '').includes('切换语言'))
  if (!btn) return null
  const r = btn.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (langBtn) {
  await page.mouse.click(langBtn.x, langBtn.y)
  await sleep(700)
  const items = await page.evaluate(() =>
    [...document.querySelectorAll('[data-slot="dropdown-menu-content"] [data-slot="dropdown-menu-item"]')].map((i) => i.textContent.trim()),
  )
  console.log('lang menu items:', JSON.stringify(items.slice(0, 4)), '... total', items.length)
  await page.screenshot({ path: 'shots/14-lang-menu.png' })
  // 选 English
  const enItem = await page.evaluate(() => {
    const el = [...document.querySelectorAll('[data-slot="dropdown-menu-item"]')].find((i) => i.textContent.includes('English'))
    if (!el) return null
    const r = el.getBoundingClientRect()
    return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
  })
  if (enItem) { await page.mouse.click(enItem.x, enItem.y); await sleep(900) }
}
const enTitle = await page.evaluate(() => document.querySelector('h1')?.textContent)
console.log('title after EN:', enTitle)
await page.screenshot({ path: 'shots/15-dashboard-en.png' })

// 切换到阿拉伯语(RTL)
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || '').includes('Switch language'))
  btn?.click()
})
await sleep(700)
const arItem = await page.evaluate(() => {
  const el = [...document.querySelectorAll('[data-slot="dropdown-menu-item"]')].find((i) => i.textContent.includes('العربية'))
  if (!el) return null
  const r = el.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (arItem) { await page.mouse.click(arItem.x, arItem.y); await sleep(900) }
const dir = await page.evaluate(() => document.documentElement.dir)
console.log('dir after AR:', dir)
await page.screenshot({ path: 'shots/16-dashboard-ar.png' })

// 回中文
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || '').includes('تبديل اللغة'))
  if (!btn) {
    // RTL 下 aria-label 也被翻译了,找不到就清 localStorage 刷新
  }
})
// 直接清 localStorage 回中文
await page.evaluate(() => localStorage.removeItem('dml_lang'))
await page.reload({ waitUntil: 'load' })
await sleep(1800)

// licenses 页(展开侧边栏 hover 验证导航)
await page.goto(BASE + '/licenses', { waitUntil: 'load' })
await sleep(1500)
await page.screenshot({ path: 'shots/17-licenses-i18n.png' })

// logo SVG 检查
const logoOk = await page.evaluate(() => {
  const svg = document.querySelector('aside svg[viewBox="0 0 1024 1024"]')
  return !!svg
})
console.log('sidebar logo svg present:', logoOk)

// 移动端
await page.setViewport({ width: 390, height: 844 })
await page.goto(BASE + '/dashboard', { waitUntil: 'load' })
await sleep(1500)
const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
console.log('mobile overflow:', overflow)
await page.screenshot({ path: 'shots/18-mobile-dashboard.png' })

console.log('\nerrors:', errors.length ? errors.slice(0, 8).join('\n') : 'none')
await browser.close()
