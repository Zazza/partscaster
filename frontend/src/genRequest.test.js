// genRequest — тело api.submit одного рендера «применить всё» с изменениями, которым нужна видеокарта
// (карточка internal-studio-draft3, условие 5, ТК1). Проверяются свойства заявки: план (приёмы, обрезка
// по длительности, голоса Vocal), стиль, текст, сид, режим плана, родитель и роль.
import { describe, it, expect } from 'vitest'
import { genRequest, trickStyle, dropVoices } from './requests.js'
import { applyTrick, sliceAbc, planTimeline } from './abcEdit.js'
import { voiceDescriptor } from './voiceLab.js'

// диалект YuE: 100 BPM, такт 4/4 = 2,4 с; 6 тактов на голос = 14,4 с плана
const ABC = [
  'X:1', 'T:', 'M:4/4', 'L:1/16', 'Q:1/4=100',
  'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
  'V: Ins clef=treble name="Ins Melody" snm="Inst."',
  'K:Dm',
  '% verse',
  'V: Vocal',
  '"Dm"d2a2g2e2d2a2g2e2|"Dm"d2a2g2e2d2a2g2e2|"Dm"d2a2g2e2d2a2g2e2|',
  'V: Ins',
  'd4d4d4d4|d4d4d4d4|d4d4d4d4|',
  '% chorus',
  'V: Vocal',
  '"F"f2a2c2a2f2a2c2a2|"F"f2a2c2a2f2a2c2a2|"F"f2a2c2a2f2a2c2a2|',
  'V: Ins',
  'e4e4e4e4|e4e4e4e4|e4e4e4e4|',
].join('\n')

const BAR_SEC = 2.4
const barsOf = (abc, voice) => (planTimeline(abc)[voice] || []).length
const hasVocalVoice = (abc) => /^V:\s*Vocal/m.test(abc)

// версия трека (renderJob уже применён): аудио 5 с — короче плана
const JOB = {
  id: 42, title: 'Песня', duration_sec: 5,
  style: 'post-punk, male baritone vocals, driving bass, 120 BPM',
  lyrics: '[verse]\nслова песни', seed: 777, cot: 'full',
}

// приёмы плана — в форме, которую копит шаг «Править» (pendingSpecs)
const OCT_UP = { kind: 'octave', dir: 'up', label: 'октава вверх', from: 0, to: 2.4, targets: [{ voice: 'Vocal', bar: 0 }] }
const REST_INS = { kind: 'rest', label: 'пауза', from: 2.4, to: 4.8, targets: [{ voice: 'Ins', bar: 1 }] }

const PARAMS = { register: 'female', rough: 0, creak: 0, delivery: 1, breath: 1, extra: 'airy' }
const VOICE = { params: PARAMS, name: 'Эфир' }

const gen = (over = {}) => ({ specs: [], voice: null, instrumental: false, ...over })
const req = (g, extra = {}) => genRequest(JOB, { baseAbc: ABC, gen: g, title: 'Песня · итог', ...extra })

describe('genRequest: только приёмы плана (specs)', () => {
  it('план — исходный с приёмами по порядку, обрезанный по длительности аудио (0..dur+1)', () => {
    const r = req(gen({ specs: [OCT_UP, REST_INS] }))
    const expected = sliceAbc(applyTrick(applyTrick(ABC, OCT_UP), REST_INS), 0, JOB.duration_sec + 1, 0)
    expect(r.abc).toBe(expected)
    // обрезан: в 6 с аудио — 3 такта, в исходном плане 6
    expect(barsOf(ABC, 'Vocal')).toBe(6)
    expect(barsOf(r.abc, 'Vocal')).toBe(Math.ceil((JOB.duration_sec + 1) / BAR_SEC))
    // приём применён: план отличается от просто обрезанного исходного
    expect(r.abc).not.toBe(sliceAbc(ABC, 0, JOB.duration_sec + 1, 0))
  })

  it('приёмы применяются по порядку: «вырезать такт» и «октава» на том же такте в разном порядке — разные планы', () => {
    const cut = { kind: 'cut', label: 'вырезать', targets: [{ voice: 'Vocal', bar: 2 }] }
    const oct = { kind: 'octave', dir: 'up', label: 'октава', targets: [{ voice: 'Vocal', bar: 2 }] }
    const a = req(gen({ specs: [cut, oct] })).abc
    const b = req(gen({ specs: [oct, cut] })).abc
    expect(a).not.toBe(b)
    expect(a).toBe(sliceAbc(applyTrick(applyTrick(ABC, cut), oct), 0, JOB.duration_sec + 1, 0))
    expect(b).toBe(sliceAbc(applyTrick(applyTrick(ABC, oct), cut), 0, JOB.duration_sec + 1, 0))
  })

  it('стиль — trickStyle(стиль версии, specs): приписка октавы, голос стиля прежний', () => {
    const r = req(gen({ specs: [OCT_UP] }))
    expect(r.style).toBe(trickStyle(JOB.style, [OCT_UP]))
    expect(r.style).toContain('octave-up')
    expect(r.style).toContain('male baritone vocals')
    expect(r.style).not.toContain('instrumental')
  })

  it('текст — текст версии; голос Vocal в плане есть', () => {
    const r = req(gen({ specs: [REST_INS] }))
    expect(r.lyrics).toBe(JOB.lyrics)
    expect(hasVocalVoice(r.abc)).toBe(true)
  })

  it('длительность аудио неизвестна (0) → план не режется', () => {
    const r = genRequest({ ...JOB, duration_sec: 0 }, { baseAbc: ABC, gen: gen({ specs: [REST_INS] }), title: 't' })
    expect(r.abc).toBe(applyTrick(ABC, REST_INS))
    expect(barsOf(r.abc, 'Vocal')).toBe(6)
  })
})

describe('genRequest: только новый голос (voice)', () => {
  it('стиль — swapVocals: дескриптор нового голоса вместо прежнего голоса стиля, прочие теги на месте', () => {
    const r = req(gen({ voice: VOICE }))
    expect(r.style).toContain(voiceDescriptor(PARAMS))
    expect(r.style).not.toContain('male baritone vocals')
    expect(r.style).toContain('post-punk')
    expect(r.style).toContain('driving bass')
    expect(r.style).toContain('120 BPM')
    expect(r.style).not.toContain('instrumental')
  })

  it('план — исходный (без приёмов), обрезанный по длительности; голос Vocal есть; текст версии', () => {
    const r = req(gen({ voice: VOICE }))
    expect(r.abc).toBe(sliceAbc(ABC, 0, JOB.duration_sec + 1, 0))
    expect(hasVocalVoice(r.abc)).toBe(true)
    expect(r.lyrics).toBe(JOB.lyrics)
  })

  it('приёмы и голос вместе: в стиле и приписка октавы, и новый голос; план с приёмами', () => {
    const r = req(gen({ specs: [OCT_UP], voice: VOICE }))
    expect(r.style).toContain(voiceDescriptor(PARAMS))
    expect(r.style).toContain('octave-up')
    expect(r.style).not.toContain('male baritone vocals')
    expect(r.abc).toBe(sliceAbc(applyTrick(ABC, OCT_UP), 0, JOB.duration_sec + 1, 0))
  })
})

describe('genRequest: «без голоса» (instrumental)', () => {
  it('план без голосов Vocal (по плану, vocals не переданы), прочие голоса и обрезка на месте', () => {
    const r = req(gen({ instrumental: true }))
    expect(hasVocalVoice(r.abc)).toBe(false)
    expect(r.abc).toMatch(/^V:\s*Ins/m)
    expect(barsOf(r.abc, 'Ins')).toBe(Math.ceil((JOB.duration_sec + 1) / BAR_SEC))
  })

  it("текст '[Instrumental]', стиль — trickStyle + ', instrumental, no vocals'", () => {
    const r = req(gen({ instrumental: true, specs: [OCT_UP] }))
    expect(r.lyrics).toBe('[Instrumental]')
    expect(r.style).toBe(trickStyle(JOB.style, [OCT_UP]) + ', instrumental, no vocals')
  })

  it('приёмы плана применяются и к инструменталу', () => {
    const r = req(gen({ instrumental: true, specs: [REST_INS] }))
    const expected = dropVoices(sliceAbc(applyTrick(ABC, REST_INS), 0, JOB.duration_sec + 1, 0), ['Vocal'])
    expect(r.abc).toBe(expected)
  })

  it('голос игнорируется: дескриптора нового голоса в стиле нет', () => {
    const r = req(gen({ instrumental: true, voice: VOICE }))
    expect(r.style).not.toContain(voiceDescriptor(PARAMS))
    expect(r.style.endsWith(', instrumental, no vocals')).toBe(true)
    expect(r.lyrics).toBe('[Instrumental]')
    expect(hasVocalVoice(r.abc)).toBe(false)
  })

  it('vocals переданы явно → убираются именно они', () => {
    const r = req(gen({ instrumental: true }), { vocals: ['Ins'] })
    expect(r.abc).not.toMatch(/^V:\s*Ins/m)
    expect(hasVocalVoice(r.abc)).toBe(true)
  })
})

describe('genRequest: сид, режим плана, родитель, роль', () => {
  it('seed версии, cot как есть, parent_id = id версии, role rebuild, заголовок из title, не черновик', () => {
    const r = req(gen({ specs: [OCT_UP] }))
    expect(r.seed).toBe(777)
    expect(r.cot).toBe('full')
    expect(r.parent_id).toBe(42)
    expect(r.role).toBe('rebuild')
    expect(r.title).toBe('Песня · итог')
    expect(r.draft || false).toBe(false)
  })

  it("cot 'off' → 'melody' (план требует full|melody)", () => {
    const r = genRequest({ ...JOB, cot: 'off' }, { baseAbc: ABC, gen: gen({ voice: VOICE }), title: 't' })
    expect(r.cot).toBe('melody')
  })

  it('без сида → 0 (воркер возьмёт сид родителя)', () => {
    const r = genRequest({ ...JOB, seed: null }, { baseAbc: ABC, gen: gen({ instrumental: true }), title: 't' })
    expect(r.seed).toBe(0)
  })

  it('вход не мутируется', () => {
    const g = gen({ specs: [OCT_UP, REST_INS], voice: VOICE })
    const job = { ...JOB }
    const snap = JSON.parse(JSON.stringify({ g, job }))
    genRequest(job, { baseAbc: ABC, gen: g, title: 't' })
    expect({ g, job }).toEqual(snap)
  })
})
