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
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 150)) })
page.on('pageerror', (e) => errors.push(String(e).slice(0, 150)))

// 登录
await page.goto(BASE, { waitUntil: 'load' })
await sleep(1600)
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

// ---------- 1. 主题切换 ----------
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || '').includes('亮色'))
  btn?.click()
})
await sleep(1400)
const theme = await page.evaluate(() => document.documentElement.dataset.theme)
console.log('theme after click:', theme)
await page.screenshot({ path: 'shots/06-dashboard-light.png' })
await page.evaluate(() => {
  const btn = [...document.querySelectorAll('button')].find((b) => (b.getAttribute('aria-label') || '').includes('暗色'))
  btn?.click()
})
await sleep(900)

// ---------- 2. 行操作 DropdownMenu(找 ⋯ 按钮) ----------
await page.goto(BASE + '/licenses', { waitUntil: 'load' })
await sleep(1500)
const dotPos = await page.evaluate(() => {
  // 找到包含 MoreHorizontal 图标的按钮(行内 ⋯)
  const btns = [...document.querySelectorAll('[data-slot="button"]')]
  const el = btns.find((b) => {
    const svg = b.querySelector('svg line')
    return svg && b.closest('tbody')
  }) || btns.find((b) => b.closest('tbody') && b.querySelector('svg'))
  if (!el) return null
  const r = el.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (dotPos) {
  await page.mouse.click(dotPos.x, dotPos.y)
  await sleep(800)
}
const menu = await page.evaluate(() => {
  const content = document.querySelector('[data-slot="dropdown-menu-content"]')
  if (!content) return { open: false }
  return { open: true, items: [...content.querySelectorAll('[data-slot="dropdown-menu-item"]')].map((i) => i.textContent.trim()) }
})
console.log('row dropdown:', JSON.stringify(menu))
await page.screenshot({ path: 'shots/05-dropdown-menu.png' })
// 选「吊销」→ AlertDialog
if (menu.open) {
  const revokeItem = await page.evaluate(() => {
    const items = [...document.querySelectorAll('[data-slot="dropdown-menu-item"]')]
    const el = items.find((i) => i.textContent.includes('吊销'))
    if (!el) return null
    const r = el.getBoundingClientRect()
    return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
  })
  if (revokeItem) {
    await page.mouse.click(revokeItem.x, revokeItem.y)
    await sleep(900)
  }
  const alert = await page.evaluate(() => {
    const d = document.querySelector('[data-slot="alert-dialog-content"]')
    if (!d) return { open: false }
    return {
      open: true,
      title: d.querySelector('[data-slot="alert-dialog-title"]')?.textContent?.trim(),
      desc: d.querySelector('[data-slot="alert-dialog-description"]')?.textContent?.trim().slice(0, 60),
      selects: d.querySelectorAll('[data-slot="select-trigger"]').length,
      actions: [...d.querySelectorAll('button')].map((b) => b.textContent.trim()),
    }
  })
  console.log('revoke alert:', JSON.stringify(alert))
  await page.screenshot({ path: 'shots/06-revoke-alert.png' })
  await page.keyboard.press('Escape')
  await sleep(500)
}

// ---------- 3. Toast(触发一个操作) ----------
await page.evaluate(() => {
  const btns = [...document.querySelectorAll('[data-slot="button"]')]
  const el = btns.find((b) => b.textContent.includes('签发 License'))
  el?.click()
})
await sleep(800)
// 填表单提交
await page.evaluate(() => {
  const inputs = [...document.querySelectorAll('[data-slot="dialog-content"] input')]
  const setVal = (el, v) => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
    setter.call(el, v)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }
  if (inputs[0]) setVal(inputs[0], '验收客户')
})
await sleep(300)
await page.evaluate(() => {
  const dlg = document.querySelector('[data-slot="dialog-content"]')
  dlg?.querySelector('form')?.requestSubmit()
})
await sleep(1500)
const result = await page.evaluate(() => {
  const dlg = document.querySelector('[data-slot="dialog-content"]')
  const keyArea = dlg?.querySelector('textarea')
  const toast = document.querySelector('.fixed.top-4.right-4 span')
  return { keyShown: keyArea ? keyArea.value.slice(0, 30) : null, toast: toast?.textContent?.slice(0, 40) }
})
console.log('issue result:', JSON.stringify(result))
await page.screenshot({ path: 'shots/07-issue-result.png' })

// ---------- 4. 移动端 Sheet ----------
await page.setViewport({ width: 390, height: 844 })
await page.goto(BASE + '/licenses', { waitUntil: 'load' })
await sleep(1500)
const menuPos = await page.evaluate(() => {
  const header = document.querySelector('header')
  const btn = header?.querySelector('button')
  if (!btn) return null
  const r = btn.getBoundingClientRect()
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 }
})
if (menuPos) {
  await page.mouse.click(menuPos.x, menuPos.y)
  await sleep(900)
}
const sheet = await page.evaluate(() => {
  const s = document.querySelector('[data-slot="sheet-content"]')
  if (!s) return { open: false }
  return { open: true, navLinks: [...s.querySelectorAll('nav a')].map((a) => a.textContent.trim()) }
})
console.log('mobile sheet:', JSON.stringify(sheet))
await page.screenshot({ path: 'shots/08-mobile-sheet.png' })

console.log('\nerrors:', errors.length ? errors.slice(0, 6).join('\n') : 'none')
await browser.close()
