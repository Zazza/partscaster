// Чистые функции abcEdit:
//   sectionRequest — заявка рендера куска группой («+ инструмент» в секцию, POST /jobs): план окна
//     со всеми голосами и тактом контекста, стиль трека с припиской инструмента, лирика
//     [Instrumental], сид (явный → родителя → 0, воркер возьмёт сид родителя), cot с планом не off,
//     производный трек parent_id + role section, не черновик;
//   beatSecAt — длина доли (с) в момент sec для выравнивания вклеек: по темпу плана, звук YuE идёт по
//     темпу заголовка (смена Q: посреди плана не исполняется — замер «темп +10%»), без темпа — 120 BPM.
import { describe, it, expect } from 'vitest'
import { sectionRequest, beatSecAt, planTimeline } from './abcEdit.js'

// 110 BPM, 4/4, L:1/16 → такт = 4 доли × 60/110 ≈ 2,18 с; у каждого голоса по 4 такта
const MULTI = [
  'X:1', 'M:4/4', 'L:1/16', 'Q:1/4=110',
  'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
  'V: Ins clef=treble name="Ins Melody" snm="Inst."',
  'K:Dm',
  '% chorus',
  'V: Vocal',
  '"Dm"z4d2f2a2a2f2d2|"Bb"z4d2f2g2g2f2d2|"F"z4c2f2a2a2g2f2|"C"z4e2g2g2e2d2e2|',
  'V: Ins',
  '"Dm"d4f2d2f2g2gfd2|"Bb"d4f2d2f2g2gfd2|"F"c4a2f2a2g2gfc2|"C"e4g2e2g2a2age2|',
].join('\n')

const BAR = 4 * 60 / 110
const flute = { id: 'flute', en: 'flute melody', stems: ['other'] }
const parent = { id: 12, title: 'Warm Wire', style: 'indie rock, baritone', seed: 4242, cot: 'full' }

describe('заявка рендера куска группой (sectionRequest)', () => {
  it('производный трек родителя: parent_id и role section, полный рендер (не черновик)', () => {
    const r = sectionRequest(parent, MULTI, flute, BAR + 0.1, 2 * BAR - 0.1)
    expect(r.parent_id).toBe(12)
    expect(r.role).toBe('section')
    expect(r.draft).toBe(false)
  })

  it('кусок без слов: лирика [Instrumental]', () => {
    expect(sectionRequest(parent, MULTI, flute, 0, BAR).lyrics).toBe('[Instrumental]')
  })

  it('название — трек и инструмент; без названия — «трек»', () => {
    const r = sectionRequest(parent, MULTI, flute, 0, BAR)
    expect(r.title).toContain('Warm Wire')
    expect(r.title).toContain('flute')
    expect(sectionRequest({ ...parent, title: '' }, MULTI, flute, 0, BAR).title).toMatch(/^трек/)
  })

  it('стиль — стиль трека с припиской инструмента', () => {
    const r = sectionRequest(parent, MULTI, flute, 0, BAR)
    expect(r.style.startsWith('indie rock, baritone')).toBe(true)
    expect(r.style).toContain('flute melody')
  })

  it('план — окно выделения и по такту контекста с каждой стороны, у всех голосов', () => {
    // окно — такт 1: в плане такты 0, 1, 2; такта 3 нет
    const r = sectionRequest(parent, MULTI, flute, BAR + 0.1, 2 * BAR - 0.1)
    const tl = planTimeline(r.abc)
    expect(Object.keys(tl).sort()).toEqual(['Ins', 'Vocal'])
    expect(tl.Vocal).toHaveLength(3)
    expect(tl.Ins).toHaveLength(3)
    expect(r.abc).toContain('"Bb"z4d2f2g2g2f2d2')   // такт окна
    expect(r.abc).not.toContain('"C"z4e2g2g2e2d2e2') // такт 3 — вне окна и контекста
    expect(r.abc).toContain('Q:1/4=110')             // заголовок плана (темп) на месте
  })

  it('сид: явный («ещё вариант») важнее сида родителя', () => {
    expect(sectionRequest(parent, MULTI, flute, 0, BAR, 99).seed).toBe(99)
  })

  it('сид: без явного — сид родителя (тот же «дубль» по таймингу)', () => {
    expect(sectionRequest(parent, MULTI, flute, 0, BAR).seed).toBe(4242)
  })

  it('сид: ни явного, ни у родителя — 0 (воркер возьмёт сид родителя)', () => {
    expect(sectionRequest({ ...parent, seed: 0 }, MULTI, flute, 0, BAR).seed).toBe(0)
    expect(sectionRequest({ ...parent, seed: undefined }, MULTI, flute, 0, BAR).seed).toBe(0)
  })

  it('cot: режим плана родителя; off и пустой → melody (рендер по плану с off воркер отклоняет)', () => {
    expect(sectionRequest({ ...parent, cot: 'full' }, MULTI, flute, 0, BAR).cot).toBe('full')
    expect(sectionRequest({ ...parent, cot: 'melody' }, MULTI, flute, 0, BAR).cot).toBe('melody')
    expect(sectionRequest({ ...parent, cot: 'off' }, MULTI, flute, 0, BAR).cot).toBe('melody')
    expect(sectionRequest({ ...parent, cot: '' }, MULTI, flute, 0, BAR).cot).toBe('melody')
  })
})

describe('длина доли в момент времени (beatSecAt)', () => {
  it('по темпу заголовка: 110 BPM → 60/110 с, 100 BPM → 0,6 с', () => {
    expect(beatSecAt(MULTI, 1)).toBeCloseTo(60 / 110, 9)
    expect(beatSecAt(MULTI.replace('Q:1/4=110', 'Q:1/4=100'), 1)).toBeCloseTo(0.6, 9)
  })

  it('в начале и на границе такта — тот же темп', () => {
    expect(beatSecAt(MULTI, 0)).toBeCloseTo(60 / 110, 9)
    expect(beatSecAt(MULTI, BAR)).toBeCloseTo(60 / 110, 9)
  })

  it('за концом плана — темп последнего такта, а не умолчание', () => {
    expect(beatSecAt(MULTI, 1000)).toBeCloseTo(60 / 110, 9)
  })

  it('план без темпа — 120 BPM (0,5 с)', () => {
    expect(beatSecAt(MULTI.replace('Q:1/4=110\n', ''), 1)).toBeCloseTo(0.5, 9)
  })

  it('пустой план — 120 BPM (0,5 с), без исключения', () => {
    expect(beatSecAt('', 3)).toBeCloseTo(0.5, 9)
  })

  it('смена темпа посреди плана не меняет долю: звук идёт по темпу заголовка', () => {
    const lines = MULTI.split('\n')
    const mid = [...lines.slice(0, -3), 'Q:1/4=140', ...lines.slice(-3)].join('\n')
    expect(beatSecAt(mid, 3 * BAR + 0.5)).toBeCloseTo(60 / 110, 9)
  })
})
