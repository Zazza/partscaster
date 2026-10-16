// Карточка internal-fix-found, условие 3: студия трека без партитуры.
// - studioReady: true, когда партитура разобрана (rollData), или когда партитуры нет (!abcFile) и ничего
//   не разбирается; иначе false (пока партитура грузится — не показываем пустую студию).
// - полоса шагов и основная часть студии (волна, «Звук», «Готово») показываются по studioReady, не по rollData;
// - пиано-ролл, приёмы по тактам и строка темпа/тональности (roll-meta) — только при rollData.
// Проверка по исходнику StudioPage.vue (как в других *.test.js, читающих .vue).
// Предположения: studioReady — computed в <script setup> вида `computed(() => <выражение>)` над рефами
// rollData, abcFile и рефом-флагом разбора (тот, что показывает t('studio.parsing')); коробки ролла и приёмов
// узнаются по подписям t('studio.roll.caption') и t('studio.trick.caption'); волна — класс wave-panel.
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it, expect } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(here, 'components/StudioPage.vue'), 'utf8')
const tplStart = src.indexOf('<template>')
const script = src.slice(0, tplStart)
const tpl = src.slice(tplStart)

// флаг «идёт разбор»: условие абзаца с t('studio.parsing')
const parsingFlag = (() => {
  const m = tpl.match(/<p\s+v-if="!?\s*([A-Za-z_$][\w$]*)"[^>]*>\s*\{\{\s*t\('studio\.parsing'\)/)
  return m && m[1]
})()

// выражение studioReady, вычисленное на заданных значениях рефов
function studioReady({ rollData, abcFile, parsing }) {
  const m = script.match(/const\s+studioReady\s*=\s*computed\(\s*\(\)\s*=>\s*([^\n]+?)\)\s*$/m)
  if (!m) throw new Error('нет const studioReady = computed(() => …)')
  const fn = new Function('rollData', 'abcFile', parsingFlag, `return (${m[1]})`)
  return !!fn({ value: rollData }, { value: abcFile }, { value: parsing })
}

// последний открывающий тег (по шаблону re) перед позицией pos
function lastTagBefore(re, pos) {
  let last = null
  for (const m of tpl.slice(0, pos).matchAll(re)) last = m[0]
  return last
}
const boxBefore = (marker) => {
  const at = tpl.indexOf(marker)
  expect(at, `нет ${marker} в шаблоне`).toBeGreaterThan(0)
  return lastTagBefore(/<div\b[^>]*class="(?:[^"]*\s)?studio-box(?:\s[^"]*)?"[^>]*>/g, at)
}

describe('StudioPage: studioReady', () => {
  it('флаг разбора партитуры найден в шаблоне', () => {
    expect(parsingFlag).toBeTruthy()
  })
  it('партитура разобрана → студия готова', () => {
    expect(studioReady({ rollData: { bars: [] }, abcFile: 'score.abc', parsing: false })).toBe(true)
  })
  it('партитуры нет и ничего не разбирается → студия готова (трек без партитуры)', () => {
    expect(studioReady({ rollData: null, abcFile: '', parsing: false })).toBe(true)
  })
  it('партитура есть, но ещё не разобрана → не готова', () => {
    expect(studioReady({ rollData: null, abcFile: 'score.abc', parsing: false })).toBe(false)
    expect(studioReady({ rollData: null, abcFile: 'score.abc', parsing: true })).toBe(false)
  })
  it('идёт разбор → не готова, даже без abcFile', () => {
    expect(studioReady({ rollData: null, abcFile: '', parsing: true })).toBe(false)
  })
})

describe('StudioPage: что видно без партитуры', () => {
  it('полоса шагов показывается по studioReady', () => {
    const nav = tpl.match(/<nav\b[^>]*class="studio-steps"[^>]*>/)
    expect(nav, 'нет <nav class="studio-steps">').toBeTruthy()
    expect(nav[0]).toContain('v-if="studioReady"')
  })
  it('основная часть студии (с волной) — под <template v-if="studioReady">, не под rollData', () => {
    const at = tpl.indexOf('wave-panel')
    expect(at).toBeGreaterThan(0)
    // открытые (не закрытые) <template …> на позиции волны, с учётом вложенности
    const open = []
    for (const m of tpl.slice(0, at).matchAll(/<template\b[^>]*>|<\/template>/g)) {
      if (m[0] === '</template>') open.pop()
      else open.push(m[0])
    }
    const conds = open.filter((x) => /v-if=/.test(x))
    expect(conds).toContain('<template v-if="studioReady">')
    expect(conds.join(' ')).not.toMatch(/rollData/)
    // и коробка волны сама по себе не требует партитуры
    expect(lastTagBefore(/<div\b[^>]*>/g, at + 'wave-panel'.length)).not.toMatch(/rollData/)
  })
  it('пиано-ролл — только при rollData', () => {
    expect(boxBefore("t('studio.roll.caption')")).toContain('v-if="rollData"')
  })
  it('приёмы по тактам — только при rollData', () => {
    expect(boxBefore("t('studio.trick.caption')")).toContain('v-if="rollData"')
  })
  it('строка темпа/тональности (roll-meta) — только при rollData', () => {
    const meta = tpl.match(/<[a-z]+\b[^>]*class="[^"]*\broll-meta\b[^"]*"[^>]*>/)
    expect(meta, 'нет элемента roll-meta').toBeTruthy()
    expect(meta[0]).toContain('v-if="rollData"')
  })
})
