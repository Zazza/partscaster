// То, что живёт только в шаблоне и стилях: проводка PedalBoard в StudioPage (по дереву разбора шаблона)
// и CSS волны сверху.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import { templateAst, findElements, dir } from './testkit/sfc.js'

describe('шаблон StudioPage и стили App.vue', () => {
  it('PedalBoard в StudioPage получает :active="dspTarget" (цель правок, не targetFor(activeStem))', () => {
    const boards = findElements(templateAst('components/StudioPage.vue'), (n) => n.tag === 'PedalBoard')
    expect(boards.length, 'PedalBoard в шаблоне StudioPage не найден').toBeGreaterThan(0)
    for (const { node } of boards) expect(dir(node, 'bind', 'active')?.trim()).toBe('dspTarget')
  })

  it('у .roll-block в App.vue overflow-x: clip, а не auto (волна сверху прилипает к странице)', () => {
    const sfc = readFileSync(new URL('./App.vue', import.meta.url), 'utf8')
    const css = [...sfc.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n')
    const rules = [...css.matchAll(/([^{}]*)\{([^{}]*)\}/g)]
      .filter((r) => r[1].split(',').some((sel) => /\.roll-block\s*$/.test(sel.trim())))
    expect(rules.length, '.roll-block не найден в стилях App.vue').toBeGreaterThan(0)
    const decls = rules.map((r) => r[2]).join(';')
    expect(decls).toMatch(/\boverflow-x\s*:\s*clip\b/)
    expect(decls).not.toMatch(/\boverflow(?:-x)?\s*:\s*(?:auto|scroll)\b/)
  })
})
