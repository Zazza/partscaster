// «Студия → продолжить с места»: подсказки к приписке стиля (CONT_HINTS) и
// сборка приписки (contStyleAdd). Тесты по спецификации задачи; проверки
// StudioPage.vue — по исходникам, как в dropOllama.test.js.
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'
import { CONT_HINTS, contStyleAdd } from './contHints.js'
import { TRICK_INSTRUMENTS, TRICK_MUTES } from './abcEdit.js'
import { continueRequest } from './requests.js'

const srcDir = dirname(fileURLToPath(import.meta.url))
const STUDIO = join(srcDir, 'components', 'StudioPage.vue')
const CYR = /[Ѐ-ӿ]/
const GROUPS = ['energy', 'sound', 'mood']

function templateOf(src) {
  const start = src.indexOf('<template')
  const end = src.lastIndexOf('</template>')
  return start < 0 || end < 0 ? '' : src.slice(start, end)
}

function scriptOf(src) {
  const m = src.match(/<script\b[^>]*>([\s\S]*?)<\/script>/g)
  return m ? m.join('\n') : ''
}

// первые подсказки разных групп — чтобы проверять порядок, не зная состава списка
const firstOf = (group) => CONT_HINTS.find((h) => h.group === group)

describe('список подсказок CONT_HINTS', () => {
  test('непустой массив записей { id, group, en }', () => {
    expect(Array.isArray(CONT_HINTS)).toBe(true)
    expect(CONT_HINTS.length).toBeGreaterThan(0)
    for (const h of CONT_HINTS) {
      expect(typeof h.id).toBe('string')
      expect(h.id.length).toBeGreaterThan(0)
      expect(typeof h.group).toBe('string')
      expect(typeof h.en).toBe('string')
    }
  })

  test('группа — только energy | sound | mood', () => {
    for (const h of CONT_HINTS) expect(GROUPS).toContain(h.group)
  })

  test('en — непустая английская фраза: латиница, без кириллицы', () => {
    for (const h of CONT_HINTS) {
      expect(h.en.trim().length).toBeGreaterThan(0)
      expect(h.en).not.toMatch(CYR)
      expect(h.en).toMatch(/[A-Za-z]/)
    }
  })

  test('id уникальны', () => {
    const ids = CONT_HINTS.map((h) => h.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  test('en уникальны', () => {
    const ens = CONT_HINTS.map((h) => h.en)
    expect(new Set(ens).size).toBe(ens.length)
  })
})

describe('подсказки не дублируют приёмы блоков выше', () => {
  test('ни один id не совпадает с id приёмов-инструментов и приёмов громкости', () => {
    const taken = new Set([...TRICK_INSTRUMENTS, ...TRICK_MUTES].map((t) => t.id))
    for (const h of CONT_HINTS) expect(taken.has(h.id), h.id).toBe(false)
  })

  test('ни одна en-фраза не совпадает с фразой приёма-инструмента', () => {
    const taken = new Set(TRICK_INSTRUMENTS.map((t) => t.en))
    for (const h of CONT_HINTS) expect(taken.has(h.en), h.en).toBe(false)
  })
})

describe('приписка к стилю contStyleAdd(selectedIds, freeText)', () => {
  test('пустой выбор и пустой текст — пустая строка', () => {
    expect(contStyleAdd([], '')).toBe('')
  })

  test('пустой выбор и пробельный текст — пустая строка', () => {
    expect(contStyleAdd([], '   \n\t ')).toBe('')
  })

  test('одна подсказка без своего текста — её английская фраза', () => {
    const h = CONT_HINTS[0]
    expect(contStyleAdd([h.id], '')).toBe(h.en)
  })

  test('только свой текст — обрезан по краям', () => {
    expect(contStyleAdd([], '  warmer tape sound \n')).toBe('warmer tape sound')
  })

  test('фразы идут в порядке списка CONT_HINTS, а не в порядке выбора', () => {
    expect(CONT_HINTS.length).toBeGreaterThanOrEqual(3)
    const [a, b, c] = CONT_HINTS
    expect(contStyleAdd([c.id, a.id, b.id], '')).toBe([a.en, b.en, c.en].join(', '))
  })

  test('подсказки разных групп — тоже по порядку списка', () => {
    const picked = GROUPS.map(firstOf).filter(Boolean)
    expect(picked.length).toBeGreaterThanOrEqual(2)
    const expected = CONT_HINTS.filter((h) => picked.includes(h)).map((h) => h.en).join(', ')
    expect(contStyleAdd(picked.map((h) => h.id).reverse(), '')).toBe(expected)
  })

  test('свой текст — после подсказок, через ", "', () => {
    const [a, b] = CONT_HINTS
    expect(contStyleAdd([b.id, a.id], '  lo-fi  ')).toBe(`${a.en}, ${b.en}, lo-fi`)
  })

  test('подсказка и пробельный текст — без хвостовой запятой', () => {
    const h = CONT_HINTS[0]
    expect(contStyleAdd([h.id], '   ')).toBe(h.en)
  })

  test('неизвестный id игнорируется', () => {
    const h = CONT_HINTS[0]
    expect(contStyleAdd(['no-such-hint'], '')).toBe('')
    expect(contStyleAdd(['no-such-hint', h.id], 'x')).toBe(`${h.en}, x`)
  })

  test('свой текст на кириллице уходит как есть, без перевода', () => {
    expect(contStyleAdd([], ' мрачнее и тяжелее ')).toBe('мрачнее и тяжелее')
    const h = CONT_HINTS[0]
    expect(contStyleAdd([h.id], 'хриплый голос')).toBe(`${h.en}, хриплый голос`)
  })

  test('все подсказки разом — все фразы списка по порядку', () => {
    const all = CONT_HINTS.map((h) => h.id).reverse()
    expect(contStyleAdd(all, '')).toBe(CONT_HINTS.map((h) => h.en).join(', '))
  })
})

describe('подписи подсказок и групп в словарях', () => {
  test('у каждой подсказки есть studio.cont.hint.<id> в ru и en; русская — с кириллицей', () => {
    for (const h of CONT_HINTS) {
      const key = `studio.cont.hint.${h.id}`
      expect(typeof RU[key], key).toBe('string')
      expect(typeof EN[key], key).toBe('string')
      expect(RU[key].trim().length, key).toBeGreaterThan(0)
      expect(EN[key].trim().length, key).toBeGreaterThan(0)
      expect(RU[key], key).toMatch(CYR)
    }
  })

  test('у каждой группы есть studio.cont.hintgroup.<group> в ru и en; русская — с кириллицей', () => {
    for (const g of new Set(CONT_HINTS.map((h) => h.group))) {
      const key = `studio.cont.hintgroup.${g}`
      expect(typeof RU[key], key).toBe('string')
      expect(typeof EN[key], key).toBe('string')
      expect(RU[key].trim().length, key).toBeGreaterThan(0)
      expect(EN[key].trim().length, key).toBeGreaterThan(0)
      expect(RU[key], key).toMatch(CYR)
    }
  })
})

describe('continueRequest принимает собранную приписку как раньше', () => {
  const job = { id: 7, title: 'T', seed: 1 }
  const sel = { from: 1, to: 2 }

  test('приписка из подсказок и своего текста доходит в styleAdd без изменений', () => {
    const add = contStyleAdd([CONT_HINTS[0].id], ' своё ')
    expect(continueRequest(job, sel, '', [], add).styleAdd).toBe(add)
  })

  test('пустая приписка — пустой styleAdd; края обрезаются', () => {
    expect(continueRequest(job, sel, '', [], contStyleAdd([], '  ')).styleAdd).toBe('')
    expect(continueRequest(job, sel, '', [], '  a, b  ').styleAdd).toBe('a, b')
  })
})

describe('StudioPage.vue: поле приписки продолжения', () => {
  const src = readFileSync(STUDIO, 'utf8')
  const tpl = templateOf(src)
  const script = scriptOf(src)

  test('предупреждение о кириллице: hasCyrillic( и ключ form.style.cyr', () => {
    expect(src).toContain('hasCyrillic(')
    expect(tpl).toMatch(/\bt\(\s*['"]form\.style\.cyr['"]/)
  })

  test('заявка продолжения собирается через contStyleAdd( из contHints.js', () => {
    expect(script).toMatch(/import\s*\{[^}]*\bcontStyleAdd\b[^}]*\}\s*from\s*['"][./]*contHints(\.js)?['"]/)
    expect(src).toContain('contStyleAdd(')
  })
})
