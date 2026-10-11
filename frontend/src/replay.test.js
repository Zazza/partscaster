// Тесты карточки internal-own-track, условие 125 «Овердаб → „Переиграть в другом стиле“» (тест-кейс ТК152):
//   - replayRequest(form, job) → {style, lyrics, gain, seed} | null:
//       стиль — жанр, партии-чипы (odPartyChips, по-английски) и «своими словами» через «, »; пустой → null;
//       голос: lyrics — текст трека, none — '[Instrumental]', own — свой текст (пустой свой → текст трека);
//       результат: cover → gain 0 (без смешивания), layer → gain в [0,1; 1];
//       интерпретация: close → сид трека, new → 0;
//   - по исходнику: StudioPage.vue без блока овердаба (нет submitOverdub / studio.overdub),
//     App.vue — пункт меню t('replay.menu') и ReplayModal, ReplayModal.vue зовёт api.submitOverdub.
// Написаны по карточке, без чтения реализации.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import { replayRequest, REPLAY_INSTRUMENTAL } from './replay.js'
import { odPartyChips } from './slotOptions.js'

const JOB = { id: 42, seed: 12345, lyrics: '[verse]\nwarm wire hums', style: 'dark rock' }

// форма модалки (интерфейс уточнён по JSDoc replay.js после написания тестов: карточка его не задаёт):
// chips — английские подписи odPartyChips, mix — 'cover'|'layer', close — «ближе к оригиналу» (иначе «новая»)
const form = (over = {}) => ({
  genre: 'post-punk',
  chips: [],
  text: '',
  voice: 'lyrics',
  ownLyrics: '',
  mix: 'cover',
  gain: 0.5,
  close: true,
  ...over,
})

describe('replayRequest: стиль', () => {
  it('только жанр → стиль = жанр', () => {
    expect(replayRequest(form(), JOB).style).toBe('post-punk')
  })

  it('жанр, партии и свой текст — через «, » в этом порядке', () => {
    const r = replayRequest(form({ chips: [odPartyChips[0].en, odPartyChips[6].en], text: 'slow, brushes on drums' }), JOB)
    expect(r.style).toBe(['post-punk', odPartyChips[0].en, odPartyChips[6].en, 'slow, brushes on drums'].join(', '))
  })

  it('без жанра — только партии', () => {
    const r = replayRequest(form({ genre: '', chips: [odPartyChips[3].en] }), JOB)
    expect(r.style).toBe(odPartyChips[3].en)
  })

  it('без жанра и партий — только свой текст (обрезанный)', () => {
    const r = replayRequest(form({ genre: '', text: '  warm tube sound  ' }), JOB)
    expect(r.style).toBe('warm tube sound')
  })

  it('пустой стиль (нет жанра, партий, текста) → null', () => {
    expect(replayRequest(form({ genre: '', chips: [], text: '' }), JOB)).toBeNull()
    expect(replayRequest(form({ genre: '', chips: [], text: '   ' }), JOB)).toBeNull()
  })
})

describe('replayRequest: голос', () => {
  it('lyrics → текст трека', () => {
    expect(replayRequest(form({ voice: 'lyrics' }), JOB).lyrics).toBe(JOB.lyrics)
  })

  it("none → '[Instrumental]' (константа REPLAY_INSTRUMENTAL)", () => {
    expect(REPLAY_INSTRUMENTAL).toBe('[Instrumental]')
    expect(replayRequest(form({ voice: 'none' }), JOB).lyrics).toBe('[Instrumental]')
  })

  it('own → свой текст', () => {
    const own = '[verse]\nstatic and gold'
    expect(replayRequest(form({ voice: 'own', ownLyrics: own }), JOB).lyrics).toBe(own)
  })

  it('own с пустым своим текстом → текст трека', () => {
    expect(replayRequest(form({ voice: 'own', ownLyrics: '' }), JOB).lyrics).toBe(JOB.lyrics)
    expect(replayRequest(form({ voice: 'own', ownLyrics: '  \n ' }), JOB).lyrics).toBe(JOB.lyrics)
  })
})

describe('replayRequest: результат (громкость)', () => {
  it('cover → gain 0 (новый трек без смешивания), какая бы громкость ни стояла', () => {
    expect(replayRequest(form({ mix: 'cover', gain: 0.7 }), JOB).gain).toBe(0)
  })

  it('layer → громкость слоя как есть внутри [0,1; 1]', () => {
    expect(replayRequest(form({ mix: 'layer', gain: 0.4 }), JOB).gain).toBeCloseTo(0.4)
    expect(replayRequest(form({ mix: 'layer', gain: 1 }), JOB).gain).toBeCloseTo(1)
    expect(replayRequest(form({ mix: 'layer', gain: 0.1 }), JOB).gain).toBeCloseTo(0.1)
  })

  it('layer → громкость за краями прижимается к [0,1; 1] (0 не превращает слой в кавер)', () => {
    const g0 = replayRequest(form({ mix: 'layer', gain: 0 }), JOB).gain
    expect(g0).toBeGreaterThanOrEqual(0.1)
    expect(g0).toBeLessThanOrEqual(1)
    expect(replayRequest(form({ mix: 'layer', gain: 0.03 }), JOB).gain).toBeCloseTo(0.1)
    expect(replayRequest(form({ mix: 'layer', gain: 1.8 }), JOB).gain).toBeCloseTo(1)
  })
})

describe('replayRequest: интерпретация (сид)', () => {
  it('ближе к оригиналу → сид трека', () => {
    expect(replayRequest(form({ close: true }), JOB).seed).toBe(12345)
  })

  it('новая интерпретация → 0', () => {
    expect(replayRequest(form({ close: false }), JOB).seed).toBe(0)
  })
})

it('replayRequest возвращает ровно {style, lyrics, gain, seed}', () => {
  const r = replayRequest(form({ mix: 'layer', gain: 0.5, voice: 'none', close: false }), JOB)
  expect(Object.keys(r).sort()).toEqual(['gain', 'lyrics', 'seed', 'style'])
})

// ---------- по исходнику ----------

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8')
const STUDIO = read('./components/StudioPage.vue')
const APP = read('./App.vue')
const MODAL = read('./components/ReplayModal.vue')

describe('исходник: овердаб ушёл из студии в действие трека', () => {
  it('StudioPage.vue не содержит блока овердаба (submitOverdub, studio.overdub)', () => {
    expect(STUDIO).not.toMatch(/submitOverdub/)
    expect(STUDIO).not.toMatch(/studio\.overdub/)
  })

  it('App.vue импортирует ReplayModal и показывает его', () => {
    expect(APP).toMatch(/import\s+ReplayModal\s+from\s+['"][^'"]*ReplayModal\.vue['"]/)
    const template = (APP.match(/<template>([\s\S]*)<\/template>/) || [])[1] || ''
    expect(template).toMatch(/<ReplayModal\b/)
  })

  it("App.vue: пункт меню с t('replay.menu')", () => {
    expect(APP).toMatch(/t\(\s*['"]replay\.menu['"]\s*\)/)
  })

  it('ReplayModal.vue отправляет заявку через api.submitOverdub и считает её через replayRequest', () => {
    expect(MODAL).toMatch(/api\.submitOverdub\s*\(/)
    expect(MODAL).toMatch(/\breplayRequest\s*\(/)
  })
})
