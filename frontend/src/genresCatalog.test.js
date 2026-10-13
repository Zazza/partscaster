// Каталог готовых стилей генерации (internal-genres-order) — по карточке задачи, как должно работать:
// без имён групп/артистов и брендов, без жанров нейрослопа, группа = течение с различимыми вариантами,
// строка стиля согласована со слотами, подсказки ручного выбора жанра не сужены.
import { describe, it, expect } from 'vitest'
import { groups } from './groups.js'
import { slotOptions } from './slotOptions.js'
import { readFileSync } from 'node:fs'

// presets.js — откалиброванные стили; в MCP они уходят отдельной группой «Пресеты» (tools/mcp-gendata.sh),
// то есть тоже «готовые стили». Файл может быть удалён реализацией — тогда проверять нечего.
const presetMods = import.meta.glob('./presets.js', { eager: true })
const calibrated = Object.values(presetMods).flatMap((m) => m.presets || [])

const items = groups.flatMap((g) => (g.items || []).map((i) => ({ ...i, group: g })))
// все готовые стили: варианты групп + откалиброванные пресеты
const allStyles = [...items, ...calibrated.map((p) => ({ ...p, group: { id: 'presets', name: 'Пресеты' } }))]

const label = (s) => `${s.group?.id}/${s?.id}`
const slotText = (s) => Object.values(s.slots || {}).filter((v) => typeof v === 'string').join(' | ')

// ТК1: имена групп/артистов и бренды (регистр не важен)
const BANNED = [
  'interpol', 'joy division', 'radiohead', 'placebo', 'white stripes', 'portishead', 'marley',
  'руки вверх', 'bondage fairies', 'offspring', 'sex pistols', 'idles', 'hank wood',
  'nintendo', 'нинтендо', 'motown', 'мотаун', 'bollywood', 'болливуд', 'game boy', 'gameboy', 'famicom',
]

// ТК2: id убранных стилей (условие 2)
const REMOVED_IDS = [
  'chillout-bg', 'lounge-chill', 'ambient-relax', 'nature-newage', 'lofi-beats', 'jazzhop', 'chillhop-guitar',
  'sleepy-tape', 'smooth-jazz', 'ruki-vverh', 'ru-estrada', 'citypop', 'disco-70', 'synthpop', 'country-classic',
  'country-ballad', 'soul', 'gospel', 'epic', 'romantic-theme', 'trap', 'reggaeton', 'bollywood-retro', 'bossa',
  'india-mantra', 'kirtan', 'lovers-rock', 'marley', 'nintendo-punk', 'nintendocore',
]

const DIFF_SLOTS = ['rhythm', 'guitars', 'keys', 'vocals', 'production']

describe('ТК1: в готовых стилях нет имён групп, артистов и брендов', () => {
  it('каталог не пуст (иначе проверки ниже пусты)', () => {
    expect(items.length).toBeGreaterThan(0)
  })

  for (const banned of BANNED) {
    it(`нет «${banned}» в имени, строке стиля, слотах и id`, () => {
      const idForm = banned.replace(/\s+/g, '-')
      for (const s of allStyles) {
        const text = [s.name, s.style, slotText(s)].join(' || ').toLowerCase()
        expect(text.includes(banned), `${label(s)}: ${text}`).toBe(false)
        const id = String(s.id || '').toLowerCase()
        expect(id.includes(banned) || id.includes(idForm), `${label(s)}: id`).toBe(false)
      }
    })
  }

  it('нет приставки NES отдельным словом (бренд; подстрока в darkness и т.п. — не она)', () => {
    for (const s of allStyles) {
      const text = [s.name, s.style, slotText(s)].join(' || ')
      expect(/\bNES\b/i.test(text), `${label(s)}: ${text}`).toBe(false)
    }
  })

  it('нет слова «вайб» (имя, строка стиля, слоты)', () => {
    for (const s of allStyles) {
      const text = [s.name, s.style, slotText(s)].join(' || ').toLowerCase()
      expect(text.includes('вайб'), `${label(s)}: ${s.name}`).toBe(false)
    }
  })

  it('имена групп-течений тоже без запрещённых имён', () => {
    for (const g of groups) {
      const t = `${g.id} ${g.name}`.toLowerCase()
      for (const b of BANNED) expect(t.includes(b), `${g.id}: ${b}`).toBe(false)
    }
  })
})

describe('ТК2: убранные стили отсутствуют, пар «-instr» нет', () => {
  it('id убранных стилей (условие 2) нет ни в группах, ни в пресетах', () => {
    const ids = new Set(allStyles.map((s) => s.id))
    const left = REMOVED_IDS.filter((id) => ids.has(id))
    expect(left).toEqual([])
  })

  it('нет стилей с id на «-instr» (пары «стиль / без слов» заменены отметкой)', () => {
    const instr = allStyles.map((s) => s.id).filter((id) => /-instr$/.test(String(id)))
    expect(instr).toEqual([])
  })

  it('id стилей уникальны во всём каталоге групп', () => {
    const ids = items.map((s) => s.id)
    const dup = ids.filter((id, i) => ids.indexOf(id) !== i)
    expect(dup).toEqual([])
  })

  it('каждый вариант группы — настоящий объект стиля с id (нет «дыр» от удалённых пресетов)', () => {
    for (const g of groups) {
      for (const i of g.items || []) {
        expect(i && typeof i === 'object', `${g.id}: пустой элемент`).toBe(true)
        expect(String(i.id || '').trim(), `${g.id}: без id`).not.toBe('')
      }
    }
  })
})

// пара стилей различима: ≥ 2 различающихся слота из DIFF_SLOTS или темп отличается на ≥ 15 BPM
function distinct(a, b) {
  const norm = (v) => String(v || '').trim().toLowerCase()
  const diff = DIFF_SLOTS.filter((k) => norm(a.slots?.[k]) !== norm(b.slots?.[k])).length
  const bpmGap = Math.abs(Number(a.bpm) - Number(b.bpm))
  return diff >= 2 || bpmGap >= 15
}

// упоминание pop-punk допустимо только как отрицание: «not pop-punk» / «no pop-punk» в той же фразе (до запятой)
function wantsPopPunk(text) {
  const phrases = String(text || '').toLowerCase().split(',')
  return phrases.some((ph) => {
    const m = ph.match(/pop[- ]punk/)
    if (!m) return false
    const before = ph.slice(0, m.index)
    return !(/\bnot\s/.test(before) || /\bno\s/.test(before))
  })
}

describe('ТК3: группа = течение с различимыми вариантами', () => {
  it('в каждой группе ≥ 2 стилей', () => {
    const small = groups.filter((g) => (g.items || []).length < 2).map((g) => g.id)
    expect(small).toEqual([])
  })

  it('варианты одной группы попарно различаются ≥ 2 слотами или темпом ≥ 15 BPM', () => {
    const same = []
    for (const g of groups) {
      const its = g.items || []
      for (let i = 0; i < its.length; i++) {
        for (let j = i + 1; j < its.length; j++) {
          if (!distinct(its[i], its[j])) same.push(`${g.id}: ${its[i].id} ≈ ${its[j].id}`)
        }
      }
    }
    expect(same).toEqual([])
  })

  it('проверка различимости сама по себе ловит двойников (самопроверка правила)', () => {
    const a = { slots: { rhythm: 'r', guitars: 'g', keys: 'k', vocals: 'v', production: 'p' }, bpm: 120 }
    expect(distinct(a, { ...a, slots: { ...a.slots, rhythm: 'x' }, bpm: 130 })).toBe(false)
    expect(distinct(a, { ...a, slots: { ...a.slots, rhythm: 'x', vocals: 'y' } })).toBe(true)
    expect(distinct(a, { ...a, bpm: 135 })).toBe(true)
    expect(wantsPopPunk('punk, not pop-punk, raw')).toBe(false)
    expect(wantsPopPunk('punk, no pop punk polish')).toBe(false)
    expect(wantsPopPunk('punk, pop-punk energy')).toBe(true)
  })

  const punk = () => groups.filter((g) => /панк/i.test(g.name) && !/пост/i.test(g.name))

  it('есть группа «Панк» (не пост-панк) и в ней ≥ 4 вариантов', () => {
    const ps = punk()
    expect(ps.length, 'группа «Панк»').toBeGreaterThan(0)
    for (const g of ps) expect((g.items || []).length, g.name).toBeGreaterThanOrEqual(4)
  })

  it('варианты «Панка» не просят pop-punk (уводят от клише YuE2), допустимо только «not/no pop-punk»', () => {
    for (const g of punk()) {
      for (const i of g.items || []) {
        expect(wantsPopPunk(i.style), `${i.id}: style`).toBe(false)
        for (const [k, v] of Object.entries(i.slots || {})) {
          expect(wantsPopPunk(v), `${i.id}: slots.${k}`).toBe(false)
        }
      }
    }
  })

  it('течение не размазано: панк и пост-панк — каждое ровно одной группой', () => {
    const post = groups.filter((g) => /пост-панк/i.test(g.name))
    expect(punk().length).toBe(1)
    expect(post.length).toBe(1)
  })
})

describe('ТК5: строка стиля согласована со слотами', () => {
  it('style содержит «<bpm> BPM» ровно со значением bpm стиля', () => {
    for (const s of items) {
      expect(Number.isFinite(Number(s.bpm)) && Number(s.bpm) > 0, `${label(s)}: bpm`).toBe(true)
      expect(String(s.style), label(s)).toContain(`${s.bpm} BPM`)
    }
  })

  it('слот genre непустой', () => {
    for (const s of items) expect(String(s.slots?.genre || '').trim(), label(s)).not.toBe('')
  })

  it('стих [Instrumental] → слот голоса «instrumental, no vocals»', () => {
    for (const s of items) {
      if (s.lyrics === '[Instrumental]') expect(s.slots?.vocals, label(s)).toBe('instrumental, no vocals')
    }
  })
})

describe('ТК6: подсказки поля «жанр» не сужены', () => {
  const en = new Set((slotOptions.genre || []).map(([, e]) => e))
  for (const g of ['country', 'bossa nova', 'disco', 'reggaeton', 'gospel', 'synthpop']) {
    it(`в подсказках жанра остаётся «${g}»`, () => {
      expect(en.has(g)).toBe(true)
    })
  }
})

describe('ТК8: данные MCP (make mcp-data) совпадают с каталогом', () => {
  it('internal/mcp/style_groups.json содержит каждую встроенную группу с теми же вариантами (id, name, style)', () => {
    const json = JSON.parse(readFileSync(new URL('../../internal/mcp/style_groups.json', import.meta.url), 'utf8'))
    const pick = (g) => ({ id: g.id, name: g.name, items: (g.items || []).map((i) => ({ id: i.id, name: i.name, style: i.style })) })
    const mcp = new Map(json.map((g) => [g.id, pick(g)]))
    for (const g of groups) expect(mcp.get(g.id), g.id).toEqual(pick(g))
    // в MCP нет встроенных групп, которых уже нет в каталоге (кроме сводной «Пресеты»)
    const ids = new Set(groups.map((g) => g.id))
    const stale = json.map((g) => g.id).filter((id) => id !== 'presets' && !ids.has(id))
    expect(stale).toEqual([])
  })
})
