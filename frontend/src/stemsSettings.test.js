// Карточка internal-no-demucs, усл. 8 — в приложении нет выбора модели дорожек, Demucs нигде не упоминается.
//
// Тест-кейсы (ТК → условие карточки):
//   ТК41 (усл. 2, 8) в словарях ru/en нет ключей settings.stemsModel* (выбора модели в «Настройках» нет);
//   ТК42 (усл. 8)  ни одно значение словарей ru/en не содержит «demucs» (без учёта регистра): время
//                  разделения одно, исторических упоминаний нет;
//   ТК43 (усл. 8)  подсказка «разделить подробнее» (desk.resplit.tip) — без «модели из настроек»
//                  (нет «настро…»/«settings»);
//   ТК44 (усл. 2, 8) «Сохранить» в «Настройках» не шлёт воркеру stems_model;
//   ТК45 (усл. 8)  разделение не установлено (roformer_available=false) — на странице появляется строка про
//                  это (текст, которого нет при roformer_available=true).
// Внешняя граница — api.js (воркер) мокается; SettingsPage монтируется настоящим, как в settingsModel.test.js.
import { vi, test, expect, beforeEach } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

vi.hoisted(() => {
  globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }
  const noop = () => {}
  globalThis.window = globalThis.window || { addEventListener: noop, removeEventListener: noop, innerWidth: 1200, innerHeight: 800, navigator: {} }
})
const m = vi.hoisted(() => ({ api: {} }))
vi.mock('./api.js', () => ({ api: m.api }))
vi.mock('./composables/useConfirm.js', () => ({ useConfirm: () => ({ askConfirm: () => {} }) }))
vi.mock('./composables/useWorkerCompat.js', () => ({ useWorkerCompat: () => ({ recheck: () => {} }) }))

const { default: SettingsPage } = await import('./components/SettingsPage.vue')

const DICTS = { ru: RU, en: EN }

const CONFIG = {
  auto_stems_instrumental: true,
  stems_auto: 'studio',
  roformer_available: true,
  cpp_model: 'q8',
  cpp_low_vram: false,
  cpp_available: false,
  cpp_models: {},
}

function setApi(cfg) {
  for (const k of Object.keys(m.api)) delete m.api[k]
  Object.assign(m.api, {
    appVersion: vi.fn(async () => 'dev'),
    agentSetup: vi.fn(async () => ({ mcp_found: true, mcp_path: '/x', snippets: { claude_code: 'a' } })),
    status: vi.fn(async () => ({ model_loaded: false })),
    workerConfig: vi.fn(async () => ({ ...cfg })),
    workerVersion: vi.fn(async () => ({ version: 'dev', stems_model: 'bs-roformer-sw', components: { roformer: cfg.roformer_available } })),
    setWorkerConfig: vi.fn(async () => {}),
    setServerURL: vi.fn(async () => {}),
  })
}

beforeEach(() => setApi(CONFIG))

/** SettingsPage на рендерере-заглушке (как settingsModel.test.js): setup и загрузка настроек идут по-настоящему.
 *  В среде node SFC собран для SSR, поэтому вывод снимается его ssrRender по состоянию смонтированной страницы;
 *  texts — текстовые куски страницы (без тегов). */
async function mountSettings() {
  const { createRenderer, h, nextTick, ssrContextKey } = await import('vue')
  const { createApp } = createRenderer({
    insert() {}, remove() {}, createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null, patchProp() {},
  })
  let inst = null
  const Host = {
    setup: () => () => h(SettingsPage, {
      'server-url': 'http://localhost:8091',
      'onUpdate:serverUrl': () => {},
      ref: (i) => { if (i && i.$.setupState) inst = i.$ },
    }),
  }
  const app = createApp(Host).provide(ssrContextKey, { modules: new Set() })
  app.mount({})
  await nextTick()
  for (let i = 0; i < 10; i++) await Promise.resolve()
  await nextTick()
  const parts = []
  SettingsPage.ssrRender(inst.proxy, (x) => parts.push(x), inst, {}, inst.props, inst.setupState, inst.data, inst.type)
  const flat = async (a) => {
    let o = ''
    for (let x of a) { x = await x; o += Array.isArray(x) ? await flat(x) : String(x) }
    return o
  }
  const html = await flat(parts)
  const texts = html.replace(/<!--[\s\S]*?-->/g, '').split(/<[^>]*>/).map((x) => x.trim()).filter(Boolean)
  return { b: inst.setupState, texts }
}

// --- ТК41 ---

test.each(Object.keys(DICTS))('%s: нет ключей settings.stemsModel*', (lang) => {
  expect(Object.keys(DICTS[lang]).filter((k) => k.startsWith('settings.stemsModel'))).toEqual([])
})

// --- ТК42 ---

test.each(Object.keys(DICTS))('%s: ни одна строка не упоминает Demucs', (lang) => {
  const bad = Object.entries(DICTS[lang]).filter(([, v]) => /demucs/i.test(String(v))).map(([k]) => k)
  expect(bad).toEqual([])
})

// --- ТК43 ---

test.each(Object.keys(DICTS))('%s: «разделить подробнее» без «модели из настроек»', (lang) => {
  const tip = DICTS[lang]['desk.resplit.tip']
  expect(tip).toBeTruthy() // кнопка у старых треков остаётся
  expect(tip).not.toMatch(/настро|settings/i)
})

// --- ТК44 ---

test('«Сохранить» не шлёт воркеру stems_model', async () => {
  const { b } = await mountSettings()
  await b.save()
  for (let i = 0; i < 10; i++) await Promise.resolve()
  expect(m.api.setWorkerConfig).toHaveBeenCalled()
  const cfg = m.api.setWorkerConfig.mock.calls[0][0]
  expect(Object.keys(cfg)).not.toContain('stems_model')
})

// --- ТК45 ---

test('разделение не установлено — на странице строка об этом', async () => {
  const ok = await mountSettings()
  setApi({ ...CONFIG, roformer_available: false })
  const missing = await mountSettings()
  const extra = missing.texts.filter((s) => !ok.texts.includes(s))
  expect(extra.length, 'при roformer_available=false страница ничего не сообщает').toBeGreaterThan(0)
  expect(extra.join('\n')).toMatch(/roformer|раздел|дорож|stem|separat/i)
})
