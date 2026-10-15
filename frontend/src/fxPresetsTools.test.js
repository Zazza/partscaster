// Пресеты опознаются по полю group и stems, а не по id.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import blocks from './fxBlocks.json'
import { fxPresets } from './fxPresets.js'
import { fromWorkerChain, fillAmp } from './fxChain.js'
import { PRESET_GROUPS } from './presetGroups.js'

const has = (p, type) => p.chain.some((b) => b.type === type)
const blocksOf = (p, type) => p.chain.filter((b) => b.type === type)
const grouped = fxPresets.filter((p) => p.group !== undefined)
// правило — один тест: список нарушителей «id: причина»
const violations = (list, check) => list.flatMap((p) => check(p).map((why) => `${p.id}: ${why}`))

// «работает на голом воркере»: ни одной ссылки на загружаемый файл (захват, IR, набор)
function needsFiles(p) {
  for (const b of p.chain) {
    for (const s of blocks[b.type]?.strings || []) {
      if (!s.asset) continue
      if (s.required) return true                  // amp.model, sampler.kit, bass.kit
      if ((b[s.id] ?? '') !== '') return true      // cab.ir / reverb.ir с файлом
    }
  }
  return false
}

// оркестровая замена мелодии (orch-lead) — не «готовая гитара»
// гитара «по нотам» (группа guitar-notes) играет сэмплами набора — ей файлы нужны по определению, здесь не она
const guitars = grouped.filter((p) => (p.stems || []).includes('guitar') && !has(p, 'amp') && p.group !== 'orch-lead' &&
  p.group !== 'guitar-notes')
// басы — группа bass-tone (синт-басы набором — в группе bass-kit)
const basses = grouped.filter((p) => p.group === 'bass-tone' && JSON.stringify(p.stems) === JSON.stringify(['bass']))

describe('готовые гитары без захвата', () => {
  it('не меньше 10 гитар с group, stems ⊇ guitar, без блока amp', () => {
    expect(guitars.length).toBeGreaterThanOrEqual(10)
  })

  it('каждая: stems — guitar/other, работает без загруженных файлов', () => {
    expect(violations(guitars, (p) => [
      ...p.stems.filter((s) => !['guitar', 'other'].includes(s)).map((s) => `дорожка ${s}`),
      ...(needsFiles(p) ? ['цепочка требует файл (захват/IR/набор)'] : []),
    ])).toEqual([])
  })

  // характерные приёмы — каждый есть хотя бы у одной готовой гитары
  it.each([
    ['пост-панк: хорус и дилей', (p) => has(p, 'chorus') && has(p, 'delay')],
    ['сёрф: пружина и тремоло', (p) => has(p, 'spring') && has(p, 'tremolo')],
    ['шугейз: флэнжер и зал', (p) => has(p, 'flanger') && has(p, 'reverb')],
    ['индастриал-метал: гейт', (p) => has(p, 'gate')],
    ['альт-рок: фейзер', (p) => has(p, 'phaser')],
    ['лоуфай-кассета: лента', (p) => has(p, 'tape')],
    ['тремоло-пульс в стерео', (p) => blocksOf(p, 'tremolo').some((b) => (b.stereo ?? 0) > 0)],
  ])('есть гитара «%s»', (_, pred) => {
    expect(guitars.some(pred)).toBe(true)
  })
})

describe('готовые басы', () => {
  it('не меньше 4 басов с group, stems [bass]', () => {
    expect(basses.length).toBeGreaterThanOrEqual(4)
  })

  it('каждый: только звук — без sampler/bass (ноты не меняет)', () => {
    expect(violations(basses, (p) => ['sampler', 'bass'].filter((t) => has(p, t)).map((t) => `блок ${t}`))).toEqual([])
  })

  it.each([
    ['мелодичный с хорусом', (p) => has(p, 'chorus')],
    ['фузз: параллельный перегруз (mix < 1 — низ сохраняется)', (p) => blocksOf(p, 'drive').some((b) => (b.mix ?? 1) < 1)],
    ['медиатор: сжатый', (p) => has(p, 'comp') || has(p, 'glue')],
    ['даб: без верха (срез верха)', (p) => blocksOf(p, 'eq').some((b) => (b.lowpass_hz ?? 0) > 0)],
  ])('есть бас «%s»', (_, pred) => {
    expect(basses.some(pred)).toBe(true)
  })
})

describe('group и названия', () => {
  it('готовых с group есть (гитары и басы)', () => {
    expect(grouped.length).toBeGreaterThanOrEqual(14)
  })

  it('у каждого group — id из PRESET_GROUPS', () => {
    expect(violations(grouped, (p) => (typeof p.group === 'string' && p.group in PRESET_GROUPS ? [] : [`группа ${p.group}`])))
      .toEqual([])
  })
})

describe('amp_hint у пресетов с усилителем', () => {
  const withAmp = fxPresets.filter((p) => has(p, 'amp'))

  it('пресеты с усилителем есть', () => {
    expect(withAmp.length).toBeGreaterThan(0)
  })

  it('у каждого amp_hint — непустой массив непустых строк', () => {
    const ok = (p) => Array.isArray(p.amp_hint) && p.amp_hint.length > 0
      && p.amp_hint.every((h) => typeof h === 'string' && h.trim() !== '')
    expect(violations(withAmp, (p) => (ok(p) ? [] : [`amp_hint ${JSON.stringify(p.amp_hint)}`]))).toEqual([])
  })

  it.each([
    ['guitar-amp-clean', ['twin', 'fender', 'clean']],
    ['guitar-amp-hot', ['jcm900', 'jcm2000', 'jcm']],
    ['guitar-amp-spring', ['ac15', 'vox']],
  ])('%s: подсказки %j', (id, want) => {
    const p = fxPresets.find((x) => x.id === id)
    expect(p, `нет пресета ${id}`).toBeTruthy()
    expect(p.amp_hint).toEqual(want)
  })

  it('новый «хай-гейн» с усилителем: подсказки od2/jvm-od/5150/recto/bug', () => {
    const want = JSON.stringify(['od2', 'jvm-od', '5150', 'recto', 'bug'])
    expect(withAmp.some((p) => JSON.stringify(p.amp_hint) === want)).toBe(true)
  })
})

describe('fillAmp(chain, amps, hints)', () => {
  const BUG = 'Bug333 Clean.nam'
  const TWIN = 'Tim R Fender TwinVerb Norm Bright.nam'
  const JCM = 'Tim R JCM2000 Crunch.nam'
  const AMPS = [{ name: BUG }, { name: TWIN }, { name: JCM }]
  const chain = () => fromWorkerChain([
    { type: 'gate' }, { type: 'amp', model: '', input_db: 6 }, { type: 'cab' }, { type: 'reverb' },
  ], blocks)
  const ampModel = (c) => c.find((b) => b.type === 'amp').params.model

  it("hints ['twin','fender'] → захват Twin", () => {
    expect(ampModel(fillAmp(chain(), AMPS, ['twin', 'fender']))).toBe(TWIN)
  })

  it("hints ['jcm900','jcm'] → первая не нашлась, вторая — JCM2000", () => {
    expect(ampModel(fillAmp(chain(), AMPS, ['jcm900', 'jcm']))).toBe(JCM)
  })

  it('раньше идёт подсказка с меньшим номером, а не захват выше в списке', () => {
    // 'bug' подходит первому захвату, но подсказка 'jcm' стоит раньше
    expect(ampModel(fillAmp(chain(), AMPS, ['jcm', 'bug']))).toBe(JCM)
  })

  it("hints ['ac15'] → совпадений нет → первый захват (Bug333)", () => {
    expect(ampModel(fillAmp(chain(), AMPS, ['ac15']))).toBe(BUG)
  })

  it('hints не задан или пуст → первый захват', () => {
    expect(ampModel(fillAmp(chain(), AMPS))).toBe(BUG)
    expect(ampModel(fillAmp(chain(), AMPS, []))).toBe(BUG)
  })

  it("регистр не важен: 'TWIN' → Twin", () => {
    expect(ampModel(fillAmp(chain(), AMPS, ['TWIN']))).toBe(TWIN)
  })

  it('amps [] → цепочка как была', () => {
    const c = chain()
    expect(fillAmp(c, [], ['twin'])).toEqual(c)
  })

  it("заполненный model ('X.nam') не трогается", () => {
    const c = fromWorkerChain([{ type: 'amp', model: 'X.nam' }, { type: 'amp', model: '' }], blocks)
    const out = fillAmp(c, AMPS, ['twin'])
    expect(out[0].params.model).toBe('X.nam')
    expect(out[1].params.model).toBe(TWIN)
  })

  it('прочие блоки и параметры усилителя не меняются', () => {
    const c = chain()
    const out = fillAmp(c, AMPS, ['twin'])
    expect(out.length).toBe(c.length)
    for (const i of [0, 2, 3]) expect(out[i]).toEqual(c[i])
    const rest = { ...out[1].params }
    const rest0 = { ...c[1].params }
    delete rest.model
    delete rest0.model
    expect(rest).toEqual(rest0)
    expect(out[1].type).toBe('amp')
    expect(out[1].on).toBe(c[1].on)
  })

  it('входная цепочка, amps и hints не мутируются', () => {
    const c = chain()
    const before = JSON.parse(JSON.stringify(c))
    const amps = JSON.parse(JSON.stringify(AMPS))
    const hints = ['twin', 'fender']
    fillAmp(c, amps, hints)
    expect(c).toEqual(before)
    expect(amps).toEqual(AMPS)
    expect(hints).toEqual(['twin', 'fender'])
  })
})

// Проверка по исходникам: своих копий fillAmp нет,
// fillAmp берётся из fxChain.js, группировка (instrumentTree) — из presetGroups.js.
describe('экраны используют общие fillAmp и instrumentTree', () => {
  const src = (rel) => readFileSync(new URL(rel, import.meta.url), 'utf8')
  it('TrackDesk и InstrumentsPage: fillAmp и instrumentTree — из общих модулей, подсказки передаются', () => {
    const bad = ['./components/TrackDesk.vue', './components/InstrumentsPage.vue'].flatMap((rel) => {
      const s = src(rel)
      return [
        [/function\s+fillAmp\s*\(/.test(s), 'своя копия fillAmp'],
        [!/import\s*\{[^}]*\bfillAmp\b[^}]*\}\s*from\s*['"]\.\.\/fxChain\.js['"]/.test(s), 'fillAmp не из fxChain.js'],
        [!/amp_hint/.test(s), 'подсказки пресета не передаются'],
        [!/import\s*\{[^}]*\binstrumentTree\b[^}]*\}\s*from\s*['"]\.\.\/presetGroups\.js['"]/.test(s), 'instrumentTree не из presetGroups.js'],
      ].filter(([hit]) => hit).map(([, why]) => `${rel}: ${why}`)
    })
    expect(bad).toEqual([])
  })
})
