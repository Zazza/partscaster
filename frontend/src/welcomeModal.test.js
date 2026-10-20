// Окно «Что это» (WelcomeModal): подпись шага lead (internal-welcome-presets, условие 2, ТК2).
// Отрисовка настоящего компонента (renderHtml: setup + шаблон, серверный рендер), шаг выбирается
// через его же go(i) до отрисовки — так проверяется то, что увидит человек, а не устройство шаблона.
// Подменён только api.js (внешняя граница: HTTP к бэкенду).
import { describe, expect, test, vi, beforeEach } from 'vitest'
import { renderHtml } from './testkit/sfc.js'
import { visibleSteps } from './welcomeSteps.js'
import clips from './welcomeClips.json'

vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
vi.mock('./api.js', () => ({ api: { stopAudio: async () => {}, playAudio: async () => {}, playFile: async () => {} } }))
const { default: WelcomeModal } = await import('./components/WelcomeModal.vue')
const { useI18n } = await import('./i18n/index.js')

const steps = visibleSteps(clips)
const indexOf = (id) => steps.findIndex((s) => s.id === id)
// окно, открытое на шаге i: тот же компонент, перед отрисовкой — переход его собственной кнопкой-шагом go(i)
const onStep = (i) => ({
  ...WelcomeModal,
  setup(props, ctx) {
    const b = WelcomeModal.setup(props, ctx)
    b.go(i)
    return b
  },
})
const html = (id) => renderHtml(onStep(indexOf(id)))
// текст без тегов и с одиночными пробелами — подпись ищется как видимый текст
const text = (h) => h.replace(/<[^>]*>/g, ' ').replace(/&nbsp;/g, ' ').replace(/\s+/g, ' ')
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const presets = clips.steps.find((s) => s.id === 'presets')
const leads = (lang) => clips.steps.filter((s) => s.lead?.[lang]).map((s) => s.lead[lang])

beforeEach(() => { useI18n().setLocale('ru') })

describe('WelcomeModal: подпись шага lead', () => {
  test('данные теста на месте: presets виден и с lead, form виден и без lead', () => {
    expect(indexOf('presets')).toBeGreaterThan(0)
    expect(presets.lead?.ru).toBeTruthy()
    expect(indexOf('form')).toBeGreaterThan(0)
    const form = clips.steps.find((s) => s.id === 'form')
    expect(form.lead?.ru || form.lead?.en || '').toBe('')
  })

  test('шаг presets (ru): подпись выводится текстом lead.ru, а не en', async () => {
    const h = await html('presets')
    expect(text(h)).toContain(text(esc(presets.lead.ru)))
    expect(text(h)).not.toContain(text(esc(presets.lead.en)))
  })

  test('шаг presets (en): подпись выводится текстом lead.en, а не ru', async () => {
    useI18n().setLocale('en')
    const h = await html('presets')
    expect(text(h)).toContain(text(esc(presets.lead.en)))
    expect(text(h)).not.toContain(text(esc(presets.lead.ru)))
  })

  test('подпись — под заголовком шага: текст lead идёт после заголовка presets', async () => {
    const t = text(await html('presets'))
    const title = text(esc(presets.title.ru))
    const lead = text(esc(presets.lead.ru))
    expect(t.indexOf(title)).toBeGreaterThanOrEqual(0)
    expect(t.indexOf(lead)).toBeGreaterThan(t.indexOf(title))
  })

  test('шаг form без lead: ни одной подписи lead нет, окно отрисовано (заголовок и пары на месте)', async () => {
    for (const lang of ['ru', 'en']) {
      useI18n().setLocale(lang)
      const h = await html('form')
      const t = text(h)
      for (const l of leads(lang)) expect(t, `${lang}: чужая подпись на form`).not.toContain(text(esc(l)))
      const form = steps[indexOf('form')]
      expect(t).toContain(text(esc(form.title[lang])))
      expect(t).toContain(text(esc(form.pairs[0].title[lang])))
    }
  })

  test('шаг без lead: на месте подписи нет пустого элемента — разметка form та же, что без поля lead', async () => {
    // окно с шагом form и с шагом form, у которого lead пустой: разметка не отличается
    const form = clips.steps.find((s) => s.id === 'form')
    const before = await html('form')
    const saved = form.lead
    form.lead = { ru: '', en: '' }
    try {
      expect(await html('form')).toBe(before)
    } finally {
      if (saved === undefined) delete form.lead
      else form.lead = saved
    }
  })
})
