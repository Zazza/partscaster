// Сборка заявок к воркеру: что воркер должен получить.
import { describe, it, expect } from 'vitest'
import { slotOptions } from './slotOptions.js'
import { planTimeline } from './abcEdit.js'
import {
  formStyle, newTrackPayload,
  trickStyle, dropVoices, vocalVoices, continueRequest,
  vocalBars, revoicePlan, revoiceRecord, stemFxRequest,
} from './requests.js'

// диалект YuE: 100 BPM, такт 4/4 = 2,4 с
const ABC = [
  'X:1', 'T:', 'M:4/4', 'L:1/16', 'Q:1/4=100',
  'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
  'V: Ins clef=treble name="Ins Melody" snm="Inst."',
  'K:Dm',
  '% intro',
  'V: Vocal',
  'z16|',
  'V: Ins',
  'd8z8|',
  '% verse',
  'V: Vocal',
  '"Dm"d2a2g2e2|',
  'V: Ins',
  'd4d4d4d4|',
  '% chorus',
  'V: Vocal',
  '"Dm"d2a2g2e2|',
  'V: Ins',
  'e4e4e4e4|',
].join('\n')

const bodyOf = (abc, voice) => {
  const out = []
  let cur = null
  for (const l of abc.split('\n')) {
    const m = l.trim().match(/^V:\s*(\S+)/)
    if (m) { cur = m[1]; continue }
    if (cur === voice && l.includes('|')) out.push(l)
  }
  return out
}

// заполненная форма нового трека: всё по умолчанию
const form = (over = {}) => ({
  title: 'Gone', lyrics: '[Verse]\nline one', noLyrics: false,
  seed: '', cot: 'full', arc: '', abc: '', temperature: '', cfg: '', soundPresetIds: [],
  ...over,
})

describe('новый трек: заявка POST /jobs (newTrackPayload)', () => {
  it('название и стих уходят как есть', () => {
    const p = newTrackPayload(form())
    expect(p.title).toBe('Gone')
    expect(p.lyrics).toBe('[Verse]\nline one')
  })

  it('строки-пометки с # не поются: вырезаются из стиха', () => {
    const p = newTrackPayload(form({ lyrics: '[Verse]\n# ударение на второй слог\nline one\n  # ещё пометка' }))
    expect(p.lyrics).not.toMatch(/#/)
    expect(p.lyrics).toContain('line one')
    expect(p.lyrics).toContain('[Verse]')
  })

  // internal-vram-context ТК7: «без слов» — заглушка ровно из одной секции, стих не уходит
  it('«без слов»: вместо стиха — ровно «[Instrumental]»', () => {
    expect(newTrackPayload(form({ noLyrics: true, lyrics: 'не петь' })).lyrics).toBe('[Instrumental]')
    expect(newTrackPayload(form({ noLyrics: true, lyrics: '' })).lyrics).toBe('[Instrumental]')
  })

  it('seed пусто — 0 (воркер выберет случайный); число из поля — числом', () => {
    expect(newTrackPayload(form({ seed: '' })).seed).toBe(0)
    expect(newTrackPayload(form({ seed: null })).seed).toBe(0)
    const p = newTrackPayload(form({ seed: '831001' }))
    expect(p.seed).toBe(831001)
    expect(typeof p.seed).toBe('number')
  })

  it('cot off без драматургии и своего плана остаётся off', () => {
    expect(newTrackPayload(form({ cot: 'off' })).cot).toBe('off')
  })

  it('cot off с драматургией повышается до melody (arc без плана воркер отклоняет)', () => {
    const p = newTrackPayload(form({ cot: 'off', arc: 'build' }))
    expect(p.cot).toBe('melody')
    expect(p.arc).toBe('build')
  })

  it('cot off со своим планом ABC повышается до melody', () => {
    const p = newTrackPayload(form({ cot: 'off', abc: ABC }))
    expect(p.cot).toBe('melody')
  })

  it('cot full/melody с планом не меняется', () => {
    expect(newTrackPayload(form({ cot: 'full', abc: ABC })).cot).toBe('full')
    expect(newTrackPayload(form({ cot: 'melody', arc: 'wave' })).cot).toBe('melody')
  })

  it('свой план уходит без пробелов по краям; пустой и из пробелов — поля abc нет', () => {
    expect(newTrackPayload(form({ abc: '\n  ' + ABC + '\n\n' })).abc).toBe(ABC)
    expect(newTrackPayload(form({ abc: '' }))).not.toHaveProperty('abc')
    const p = newTrackPayload(form({ abc: '  \n ', cot: 'off' }))
    expect(p).not.toHaveProperty('abc')
    expect(p.cot).toBe('off')   // пробелы — не план: повышать незачем
  })

  it('без драматургии arc пустой', () => {
    expect(newTrackPayload(form()).arc).toBe('')
  })

  // internal-vram-context ТК7: потолок длины решает воркер (по карте) — форма max_tokens не шлёт вовсе
  it('поля max_tokens нет: со словами и без', () => {
    for (const noLyrics of [false, true]) {
      const p = newTrackPayload(form({ noLyrics }))
      expect('max_tokens' in p, `noLyrics=${noLyrics}`).toBe(false)
    }
  })

  it('характер «как обычно» не отправляется — воркер решает сам', () => {
    const p = newTrackPayload(form())
    expect(p).not.toHaveProperty('temperature')
    expect(p).not.toHaveProperty('cfg')
  })

  it('выбранный характер уходит числами', () => {
    const p = newTrackPayload(form({ temperature: '1.2', cfg: '2.5' }))
    expect(p.temperature).toBe(1.2)
    expect(p.cfg).toBe(2.5)
  })

  it('пресеты звука — у полного трека, копией списка', () => {
    const ids = [3, 7]
    const p = newTrackPayload(form({ soundPresetIds: ids }))
    expect(p.sound_preset_ids).toEqual([3, 7])
    ids.push(9)                          // форма поменялась после отправки — заявка нет
    expect(p.sound_preset_ids).toEqual([3, 7])
  })

  it('черновик пресетов звука не получает (воркер: с draft — 422)', () => {
    const p = newTrackPayload(form({ soundPresetIds: [3] }), { draft: true })
    expect(p.draft).toBe(true)
    expect(p).not.toHaveProperty('sound_preset_ids')
  })

  it('без выбранных пресетов поля sound_preset_ids нет', () => {
    expect(newTrackPayload(form({ soundPresetIds: [] }))).not.toHaveProperty('sound_preset_ids')
  })

  it('полный трек — без поля draft', () => {
    expect(newTrackPayload(form())).not.toHaveProperty('draft')
  })
})

// пара «русский → английский» из подсказок слота: проверка словаря без знания конкретных строк
const [genreRu, genreEn] = slotOptions.genre.find(([ru, en]) => /[а-яё]/i.test(ru) && ru !== en)
const [langRu, langEn] = slotOptions.language.find(([ru, en]) => /[а-яё]/i.test(ru) && ru !== en)

describe('стиль формы (formStyle)', () => {
  it('«стиль одной строкой» перекрывает слоты и стойку (без пробелов по краям)', () => {
    expect(formStyle('  dark synthwave, 120 BPM ', { genre: genreRu, bpm: 90 }, 'tube amp')).toBe('dark synthwave, 120 BPM')
  })

  it('пустая строка из пробелов не перекрывает форму', () => {
    expect(formStyle('   ', { genre: 'punk' }, '')).toBe('punk')
  })

  it('значения из списков уходят по-английски, порядок — язык перед жанром, независимо от ввода', () => {
    expect(formStyle('', { genre: genreRu, language: langRu }, '')).toBe(`${langEn}, ${genreEn}`)
  })

  it('русское значение из списка распознаётся без учёта регистра и пробелов', () => {
    expect(formStyle('', { genre: '  ' + genreRu.toUpperCase() + ' ' }, '')).toBe(genreEn)
  })

  it('свой текст в слоте — как есть; пустые слоты пропускаются', () => {
    expect(formStyle('', { genre: 'dreamy shoegaze', mood: '', vocals: '  ' }, '')).toBe('dreamy shoegaze')
  })

  it('стойка — после слотов, темп — последним «N BPM»', () => {
    expect(formStyle('', { genre: 'punk', bpm: 129 }, 'fuzz guitar')).toBe('punk, fuzz guitar, 129 BPM')
  })

  it('пустая форма — пустой стиль; темп без значения не пишется', () => {
    expect(formStyle('', {}, '')).toBe('')
    expect(formStyle('', { bpm: null }, '')).toBe('')
  })
})

describe('приписка приёмов к стилю (trickStyle)', () => {
  const up = { kind: 'octave', dir: 'up' }

  it('без приёмов, которые план держит слабо, стиль трека не меняется', () => {
    expect(trickStyle('indie rock', [])).toBe('indie rock')
    expect(trickStyle('indie rock', [{ kind: 'rest' }])).toBe('indie rock')
  })

  it('голос ±октава — припиской через запятую, без двойной запятой', () => {
    const s = trickStyle('indie rock,', [up])
    expect(s.startsWith('indie rock, ')).toBe(true)
    expect(s).not.toMatch(/,\s*,/)
    expect(s.length).toBeGreaterThan('indie rock, '.length)
  })

  it('пустой стиль трека — только приписка', () => {
    const s = trickStyle('', [up])
    expect(s).toBe(trickStyle(undefined, [up]))
    expect(s).not.toMatch(/^[,\s]/)
    expect(s.length).toBeGreaterThan(0)
  })
})

describe('инструментал: план без голоса (dropVoices, vocalVoices)', () => {
  it('голоса с vocal в имени — вокальные (без учёта регистра); пусто — пусто', () => {
    expect(vocalVoices(['Vocal', 'Ins', 'vocal2', 'Bass'])).toEqual(['Vocal', 'vocal2'])
    expect(vocalVoices(undefined)).toEqual([])
    expect(vocalVoices(['Ins'])).toEqual([])
  })

  it('убранный голос пропадает целиком: объявление и все его такты', () => {
    const out = dropVoices(ABC, ['Vocal'])
    expect(out).not.toMatch(/Vocal/)
    expect(Object.keys(planTimeline(out))).toEqual(['Ins'])
  })

  it('остальные голоса, заголовок и метки секций — на месте и в том же порядке', () => {
    const out = dropVoices(ABC, ['Vocal'])
    expect(bodyOf(out, 'Ins')).toEqual(bodyOf(ABC, 'Ins'))
    for (const h of ['X:1', 'M:4/4', 'L:1/16', 'Q:1/4=100', 'K:Dm', '% intro', '% verse', '% chorus']) {
      expect(out.split('\n')).toContain(h)
    }
    expect(planTimeline(out).Ins).toEqual(planTimeline(ABC).Ins)   // тайминг музыки тот же
  })

  it('метка секции сразу после убранного голоса не теряется', () => {
    const abc = 'X:1\nK:C\n% verse\nV: Ins\nCDEF|\nV: Vocal\ncdef|\n% chorus\nV: Ins\nGABc|\nV: Vocal\ngabc|'
    const out = dropVoices(abc, ['Vocal'])
    expect(out).toBe('X:1\nK:C\n% verse\nV: Ins\nCDEF|\n% chorus\nV: Ins\nGABc|')
  })

  it('пустой список — план без изменений', () => {
    expect(dropVoices(ABC, [])).toBe(ABC)
  })

  it('имя голоса — целиком: Vocal не задевает Vocal2', () => {
    const two = ABC.replace(/V: Ins/g, 'V: Vocal2')
    const out = dropVoices(two, ['Vocal'])
    expect(bodyOf(out, 'Vocal2')).toEqual(bodyOf(ABC, 'Ins'))
    expect(bodyOf(out, 'Vocal')).toEqual([])
  })
})

describe('«заново с места»: заявка /continue (continueRequest)', () => {
  const job = { id: 41, title: 'Gone', seed: 777 }
  const sel = { from: 12.5, to: 20 }

  it('продолжение этого трека с начала выделения, сид 0 — другой дубль от воркера', () => {
    const q = continueRequest(job, sel, ABC, [], '')
    expect(q.jobId).toBe(41)
    expect(q.from).toBe(12.5)
    expect(q.seed).toBe(0)
  })

  it('без приёмов — план пустой (воркер берёт план исходника)', () => {
    expect(continueRequest(job, sel, ABC, [], '').abc).toBe('')
  })

  it('с приёмами — изменённый план: такты на месте, приём применён', () => {
    const specs = [{ kind: 'rest', targets: [{ voice: 'Ins', bar: 1 }] }]
    const q = continueRequest(job, sel, ABC, specs, '')
    expect(q.abc).not.toBe('')
    expect(q.abc).not.toBe(ABC)
    expect(bodyOf(q.abc, 'Ins')[1]).not.toContain('d4d4d4d4')   // такт 1 голоса Ins замолчал
    expect(bodyOf(q.abc, 'Vocal')).toEqual(bodyOf(ABC, 'Vocal'))
  })

  it('«что изменить в звучании» — без пробелов по краям; пусто — пусто', () => {
    expect(continueRequest(job, sel, ABC, [], '  electric guitar enters \n').styleAdd).toBe('electric guitar enters')
    expect(continueRequest(job, sel, ABC, [], '   ').styleAdd).toBe('')
  })
})

describe('«перепеть с места» (vocalBars, revoicePlan, revoiceRecord)', () => {
  const sel = { from: 3, to: 6 }
  const octave = { kind: 'octave', dir: 'up', label: 'голос выше', targets: [{ voice: 'Vocal', bar: 1 }] }
  const rest = { kind: 'rest', label: 'провал', targets: [{ voice: 'Ins', bar: 1 }] }

  it('планы с одинаковым числом вокальных тактов совпадают, с другим — нет', () => {
    expect(vocalBars(ABC)).toBe(vocalBars(ABC.replace('d8z8|', 'z16|')))   // правка не-вокального голоса не мешает
    const longer = ABC + '\nV: Vocal\nz16|'
    expect(vocalBars(longer)).not.toBe(vocalBars(ABC))
  })

  it('план источника получает только приёмы голоса; прочие (провал по музыке) не применяются', () => {
    const q = revoicePlan(ABC, ABC, [octave, rest], sel, 'дубль')
    expect(bodyOf(q.abc, 'Ins')).toEqual(bodyOf(ABC, 'Ins'))       // провал не применён
    expect(bodyOf(q.abc, 'Vocal')[1]).not.toBe(bodyOf(ABC, 'Vocal')[1])  // октава применена
  })

  it('подпись — названия приёмов голоса; без них — подпись по умолчанию', () => {
    expect(revoicePlan(ABC, ABC, [octave, rest], sel, 'дубль').what).toBe('голос выше')
    expect(revoicePlan(ABC, ABC, [rest], sel, 'дубль').what).toBe('дубль')
  })

  it('доля для выравнивания — по темпу плана версии в начале выделения (100 BPM → 0,6 с)', () => {
    expect(revoicePlan(ABC, ABC, [], sel, 'дубль').beat).toBeCloseTo(0.6, 6)
    const fast = ABC.replace('Q:1/4=100', 'Q:1/4=120')
    expect(revoicePlan(ABC, fast, [], sel, 'дубль').beat).toBeCloseTo(0.5, 6)
  })

  it('запись дубля: версия — родитель, дубль — и вложение, и источник голоса, окно — выделение', () => {
    const job = { id: 41, title: 'Gone' }
    const rec = revoiceRecord(job, 99, sel, 0.6, 'голос выше')
    expect(rec).toMatchObject({ parent: 41, child: 99, voiceSrc: 99, from: 3, to: 6, beat: 0.6 })
    expect(rec.title).toContain('Gone')
    expect(rec.title).toContain('голос выше')
    expect(rec.title).toContain('#99')
    expect(revoiceRecord({ id: 41 }, 99, sel, 0.6, 'x').title).toContain('трек')
  })
})

describe('эффект на дорожку: запись реестра (stemFxRequest)', () => {
  it('окно — выделение', () => {
    expect(stemFxRequest('vocals', 'megaphone', { mix: 0.5 }, { from: 10, to: 14 }))
      .toEqual({ stem: 'vocals', chain: 'megaphone', params: { mix: 0.5 }, from: 10, to: 14 })
  })

  it('без выделения — вся дорожка (0…0)', () => {
    const r = stemFxRequest('drums', 'tape', {}, null)
    expect(r.from).toBe(0)
    expect(r.to).toBe(0)
  })

  it('крутилки — копией: правка после отправки запись не меняет; нет крутилок — {}', () => {
    const params = { mix: 0.5 }
    const r = stemFxRequest('vocals', 'megaphone', params, null)
    params.mix = 1
    expect(r.params).toEqual({ mix: 0.5 })
    expect(stemFxRequest('vocals', 'megaphone', null, null).params).toEqual({})
  })
})

describe('фикстура', () => {
  it('3 такта у каждого голоса, такт 2,4 с', () => {
    const tl = planTimeline(ABC)
    expect(tl.Vocal).toHaveLength(3)
    expect(tl.Ins[1].start).toBeCloseTo(2.4, 6)
  })
})
