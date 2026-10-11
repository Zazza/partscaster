// Тесты карточки internal-own-track, «Ревью s15» (ТК154, ТК155). Поведение — в модулях и их тестах:
//   123а (кусок играет с from − 4 с)          — playFragment, jobWatchStep: fragmentStart.test.js;
//   123б, 123в, 123ж (заявка куска, импорт)   — fragmentRequest, fragmentWindow: fragmentRequest.test.js;
//   120а (доска педалей не затирается)        — usePedalBoardState: composables/usePedalBoardState.test.js;
//   120б (трек без дорожек держит цель)       — targetChoice: activeTrack.test.js;
//   120в (дорожки педалей с «барабанами»)     — PEDAL_STEMS: composables/usePedalBoardState.test.js.
// Здесь — то, что живёт только в шаблоне и стилях: проводка PedalBoard в StudioPage (по дереву разбора шаблона)
// и CSS волны сверху.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import { templateAst, findElements, dir } from './testkit/sfc.js'

describe('кросс-ревью r1 (ТК155)', () => {
  it('120в: PedalBoard в StudioPage получает :active="dspTarget" (цель правок, не targetFor(activeStem))', () => {
    const boards = findElements(templateAst('components/StudioPage.vue'), (n) => n.tag === 'PedalBoard')
    expect(boards.length, 'PedalBoard в шаблоне StudioPage не найден').toBeGreaterThan(0)
    for (const { node } of boards) expect(dir(node, 'bind', 'active')?.trim()).toBe('dspTarget')
  })

  it('121а: у .roll-block в App.vue overflow-x: clip, а не auto (волна сверху прилипает к странице)', () => {
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
