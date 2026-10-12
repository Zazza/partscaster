// Заявка «сыграть кусок с правками» — fragmentWindow / fragmentRequest
// из requests.js, их зовёт renderFragment в StudioPage.vue:
//   кусок от начала (from < 1 с): обычный рендер POST /jobs с role 'fragment', без cont_from, бюджет шагов
//          max_tokens = (to + 2) · 25 (до конца куска плюс хвост 2 с, 25 шагов на секунду), по полному плану;
//          иначе — /jobs/{id}/fragment с окном куска и сидом трека;
//   кусок получает приписку стиля приёмов (голос ±октава);
//   импорт (без шагов модели) не запускает рендер, а даёт понятную ошибку.
import { describe, it, expect } from 'vitest'
import { fragmentRequest, fragmentWindow } from './requests.js'
import { continuationPlan, trickStyleSuffix } from './abcEdit.js'

const BASE = 'X:1\nT:t\nM:4/4\nL:1/8\nQ:1/4=120\nK:C\nV:Vocal\nCDEF GABc|\n'
const JOB = { id: 9, title: 'Песня', style: 'post-punk, male vocal', lyrics: '[verse]\nla', seed: 42, cot: 'melody' }
const OCT = { kind: 'octave', dir: 'up', from: 0, to: 4, targets: [] }

describe('fragmentWindow — окно куска', () => {
  it('выделение важнее приёмов', () => {
    expect(fragmentWindow({ from: 10, to: 20 }, [{ from: 1, to: 2 }], false)).toEqual({ from: 10, to: 20 })
  })

  it('без выделения — последний накопленный приём', () => {
    expect(fragmentWindow(null, [{ from: 1, to: 2 }, { from: 30, to: 38 }], false)).toEqual({ from: 30, to: 38 })
  })

  it('ни выделения, ни приёмов — нечего играть (null)', () => {
    expect(fragmentWindow(null, [], false)).toBeNull()
  })

  it('импорт: ошибка «нет шагов модели» до любой заявки', () => {
    expect(fragmentWindow({ from: 10, to: 20 }, [], true)).toEqual({ noTokens: true })
  })
})

describe('fragmentRequest — кусок от начала трека: обычный рендер', () => {
  it('from < 1 с → POST /jobs: role fragment, без cont_from, max_tokens = (to + 2) · 25, по полному плану', () => {
    const r = fragmentRequest(JOB, 0.5, 10, BASE, [])
    expect(r.fragment).toBeUndefined()
    expect(r.submit).toEqual({
      title: 'Песня · кусок', style: 'post-punk, male vocal', lyrics: JOB.lyrics, seed: 42, cot: 'melody',
      abc: BASE, max_tokens: 300, parent_id: 9, role: 'fragment',
    })
    expect(r.submit).not.toHaveProperty('cont_from')
  })

  it('дробный конец куска — бюджет округляется до целого шага', () => {
    expect(fragmentRequest(JOB, 0, 7.31, BASE, []).submit.max_tokens).toBe(Math.round(9.31 * 25))
  })

  it('cot off → melody (план нужно петь), сид 0 у трека без сида, название без заголовка — «трек · кусок»', () => {
    const r = fragmentRequest({ ...JOB, cot: 'off', seed: 0, title: '' }, 0, 4, BASE, [])
    expect(r.submit.cot).toBe('melody')
    expect(r.submit.seed).toBe(0)
    expect(r.submit.title).toBe('трек · кусок')
  })

  it('с приёмами: план с правками и приписка стиля', () => {
    const r = fragmentRequest(JOB, 0, 4, BASE, [OCT])
    const plan = continuationPlan(BASE, [OCT])
    expect(r.submit.abc).toBe(plan || BASE)
    expect(r.submit.style).toBe('post-punk, male vocal, ' + trickStyleSuffix([OCT]))
  })

  it('ровно 1 с — уже продолжение, а не рендер с начала', () => {
    expect(fragmentRequest(JOB, 1, 5, BASE, []).submit).toBeUndefined()
  })
})

describe('fragmentRequest — кусок с середины: /fragment', () => {
  it('окно куска, сид трека, без приёмов — пустой план и пустая приписка', () => {
    expect(fragmentRequest(JOB, 30, 40, BASE, [])).toEqual({
      fragment: { jobId: 9, from: 30, to: 40, seed: 42, abc: '', styleAdd: '' },
    })
  })

  it('с приёмами — изменённый план и приписка стиля приёмов', () => {
    const { fragment } = fragmentRequest(JOB, 30, 40, BASE, [OCT])
    expect(fragment.abc).toBe(continuationPlan(BASE, [OCT]))
    expect(fragment.styleAdd).toBe(trickStyleSuffix([OCT]))
    expect(fragment.styleAdd).not.toBe('')
  })
})
