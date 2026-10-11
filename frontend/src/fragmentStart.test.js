// Тесты карточки internal-own-track, уточнение 123а: готовый «кусок с правками» играет с from − 4 с;
// окно — запомненное from или cont_from джобы; начало раньше 4 с — с 0.
//   - fragmentStart(fj, lead = 4) из results.js — секунда старта проигрывания;
//   - сценарий слияния: строка списка джоб (без from) приходит поверх запомненного объекта куска —
//     окно должно пережить обновление, поэтому шаг опроса (jobWatchStep, его зовёт watchJob в StudioPage.vue)
//     сливает объект, а не заменяет;
//   - playFragment(api, fj, lead) из results.js — проигрывание готового куска (его зовёт StudioPage.vue).
// Написаны по карточке, без чтения реализации.
import { describe, it, expect, vi } from 'vitest'
import { fragmentStart, playFragment } from './results.js'
import { jobWatchStep } from './jobWatch.js'

describe('fragmentStart — старт куска за lead секунд до правки (123а)', () => {
  it('from 30 → 26', () => {
    expect(fragmentStart({ from: 30 })).toBe(26)
  })

  it('только cont_from 30 → 26', () => {
    expect(fragmentStart({ cont_from: 30 })).toBe(26)
  })

  it('from важнее cont_from', () => {
    expect(fragmentStart({ from: 30, cont_from: 50 })).toBe(26)
    expect(fragmentStart({ from: 50, cont_from: 30 })).toBe(46)
  })

  it('начало раньше lead — с 0', () => {
    expect(fragmentStart({ from: 3 })).toBe(0)
    expect(fragmentStart({ from: 0 })).toBe(0)
  })

  it('from ровно lead → 0', () => {
    expect(fragmentStart({ from: 4 })).toBe(0)
  })

  it('нет from и cont_from → 0', () => {
    expect(fragmentStart({})).toBe(0)
    expect(fragmentStart({ id: 7, status: 'done', audio_file: 'audio.flac' })).toBe(0)
  })

  it('null / undefined → 0', () => {
    expect(fragmentStart(null)).toBe(0)
    expect(fragmentStart(undefined)).toBe(0)
  })

  it('свой lead 2 → from − 2', () => {
    expect(fragmentStart({ from: 30 }, 2)).toBe(28)
    expect(fragmentStart({ cont_from: 30 }, 2)).toBe(28)
    expect(fragmentStart({ from: 1 }, 2)).toBe(0)
  })
})

describe('окно куска переживает обновление строкой списка джоб (то, что ломалось)', () => {
  const initial = () => ({ id: 7, status: 'queued', from: 30, to: 40 })
  const row = () => ({ id: 7, status: 'done', audio_file: 'audio.flac' })

  it('слияние {...запомненное, ...строка} сохраняет окно → 26', () => {
    const merged = { ...initial(), ...row() }
    expect(merged.status).toBe('done')
    expect(merged.audio_file).toBe('audio.flac')
    expect(fragmentStart(merged)).toBe(26)
  })

  it('замена объекта строкой теряет окно → 0 (поэтому нужно слияние)', () => {
    const replaced = row()
    expect(fragmentStart(replaced)).toBe(0)
  })
})

describe('jobWatchStep — шаг опроса джобы в студии: строка списка сливается с запомненным', () => {
  const initial = () => ({ id: 7, status: 'queued', from: 30, to: 40 })
  const row = () => ({ id: 7, status: 'done', audio_file: 'audio.flac' })

  it('окно куска переживает строку списка: from/to на месте, поля строки поверх, fragmentStart → 26', () => {
    const step = jobWatchStep(initial(), [{ id: 3, status: 'done' }, row()])
    expect(step.next).toEqual({ id: 7, status: 'done', audio_file: 'audio.flac', from: 30, to: 40 })
    expect(step.row).toEqual(row())
    expect(fragmentStart(step.next)).toBe(26)
  })

  it('done — только на шаге, где джоба стала готовой', () => {
    expect(jobWatchStep(initial(), [row()]).done).toBe(true)
    expect(jobWatchStep({ ...initial(), status: 'done' }, [row()]).done).toBe(false)
    expect(jobWatchStep(initial(), [{ id: 7, status: 'running' }]).done).toBe(false)
  })

  it('строки нет в списке, список пуст или объект без id → null (ничего не меняется)', () => {
    expect(jobWatchStep(initial(), [{ id: 8, status: 'done' }])).toBeNull()
    expect(jobWatchStep(initial(), null)).toBeNull()
    expect(jobWatchStep(null, [row()])).toBeNull()
    expect(jobWatchStep({ id: null, status: 'starting' }, [row()])).toBeNull()
  })

  it('вход не меняется', () => {
    const tracked = Object.freeze(initial())
    const jobs = Object.freeze([Object.freeze(row())])
    expect(() => jobWatchStep(tracked, jobs)).not.toThrow()
    expect(tracked).toEqual(initial())
  })
})

describe('playFragment — готовый кусок играет за lead секунд до правки (123а)', () => {
  const apiStub = () => {
    const calls = []
    return {
      calls,
      playAudio: vi.fn(async (id) => { calls.push(['play', id]) }),
      seekAudio: vi.fn(async (at) => { calls.push(['seek', at]) }),
    }
  }

  it('кусок from 30 → трек куска, потом перемотка на 26', async () => {
    const api = apiStub()
    await playFragment(api, { id: 7, from: 30 }, 4)
    expect(api.calls).toEqual([['play', 7], ['seek', 26]])
  })

  it('окно из cont_from строки джобы', async () => {
    const api = apiStub()
    await playFragment(api, { id: 7, cont_from: 50 }, 4)
    expect(api.calls).toEqual([['play', 7], ['seek', 46]])
  })

  it('начало раньше lead или окна нет — с начала, без перемотки', async () => {
    for (const fj of [{ id: 7, from: 3 }, { id: 7 }]) {
      const api = apiStub()
      await playFragment(api, fj, 4)
      expect(api.calls).toEqual([['play', 7]])
    }
  })
})
