// Страница настроек: модель адреса воркера. Регрессия: defineModel('serverURL')
// не сопоставлялся с v-model:server-url родителя (attr «server-url» камелячится в
// «serverUrl», а не «serverURL») — поле адреса приходило пустым, и «Сохранить»
// падал с TypeError reading 'trim'. Монтируем настоящим child-компонентом с attr
// ровно теми же ключами, что компилирует шаблон App.vue.
import { vi, test, expect, beforeEach } from 'vitest'

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

const CONFIG = {
  auto_stems_instrumental: true,
  stems_pref: 'htdemucs',
  stems_model: 'htdemucs',
  roformer_available: true,
  engine: 'torch',
  cpp_model: 'q8',
  cpp_low_vram: false,
  cpp_available: false,
  cpp_models: {},
}

beforeEach(() => {
  for (const k of Object.keys(m.api)) delete m.api[k]
  Object.assign(m.api, {
    appVersion: vi.fn(async () => 'dev'),
    agentSetup: vi.fn(async () => ({ mcp_found: true, mcp_path: '/x', snippets: { claude_code: 'a' } })),
    status: vi.fn(async () => ({ model_loaded: false })),
    workerConfig: vi.fn(async () => ({ ...CONFIG })),
    setWorkerConfig: vi.fn(async () => {}),
    setServerURL: vi.fn(async () => {}),
  })
})

/** SettingsPage как настоящий child: ключи пропсов — как у скомпилированного
 *  v-model:server-url (attr «server-url», слушатель «onUpdate:serverUrl»). */
async function mountSettings(url) {
  const { createRenderer, h, nextTick } = await import('vue')
  const { createApp } = createRenderer({
    insert() {}, remove() {}, createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null, patchProp() {},
  })
  let b = null
  const Host = {
    setup: () => () => h(SettingsPage, {
      'server-url': url,
      'onUpdate:serverUrl': () => {},
      ref: (inst) => { if (inst && inst.$.setupState) b = inst.$.setupState },
    }),
  }
  const { ssrContextKey } = await import('vue')
  const app = createApp(Host).provide(ssrContextKey, { modules: new Set() })
  app.mount({})
  await nextTick()
  for (let i = 0; i < 10; i++) await Promise.resolve()
  return b
}

test('адрес воркера доходит до страницы настроек через v-model:server-url', async () => {
  const b = await mountSettings('http://localhost:8091')
  expect(b.serverURL).toBe('http://localhost:8091')
})

test('«Сохранить» с выбранным RoFormer не падает (regression: TypeError reading trim)', async () => {
  const b = await mountSettings('http://localhost:8091')
  b.stemsModel = 'roformer'
  await b.save()
  for (let i = 0; i < 10; i++) await Promise.resolve()
  expect(String(b.err)).not.toContain('TypeError')
  // что отправили воркеру — с адресом и RoFormer
  const cfg = m.api.setWorkerConfig.mock.calls[0][0]
  expect(cfg.stems_model).toBe('roformer')
  // internal-drop-ollama: раздела Ollama в настройках нет — воркеру ollama_* не уходят
  expect(Object.keys(cfg).filter((k) => k.startsWith('ollama'))).toEqual([])
  expect(m.api.setServerURL).toHaveBeenCalledWith('http://localhost:8091')
})

test('без адреса от родителя модель — пустая строка, а не undefined', async () => {
  const b = await mountSettings(undefined)
  await b.save()
  expect(String(b.err)).not.toContain('TypeError')
})
