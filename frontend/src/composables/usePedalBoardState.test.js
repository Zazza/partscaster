// Доска педалей переживает перезапуск студии: сохранённая доска читается сразу,
// а не после монтирования, и слежение за активной дорожкой (immediate) не записывает пустую доску поверх
// сохранённой. Активная дорожка студии задаёт дорожку доски, если педали на неё ставятся (и «барабаны»).
import { describe, it, expect, beforeEach } from 'vitest'
import { nextTick, ref } from 'vue'
import { usePedalBoardState } from './usePedalBoardState.js'
import { PEDAL_STEMS } from '../pedals.js'
import { MemStorage } from '../testkit/sfc.js'

const KEY = 'yue_studio_state'
const saved = [{ chain: 'fuzz', params: { gain: 3 }, off: false }]
const stored = () => JSON.parse(globalThis.localStorage.getItem(KEY))

beforeEach(() => {
  globalThis.localStorage = new MemStorage({ [KEY]: JSON.stringify({ pedals: { 5: { board: saved, stem: 'bass' } }, other: 1 }) })
})

describe('usePedalBoardState', () => {
  it('сохранённая доска и дорожка трека — сразу при создании', () => {
    const { board, stem } = usePedalBoardState(() => 5, () => null, PEDAL_STEMS)
    expect(board.value).toEqual(saved)
    expect(stem.value).toBe('bass')
  })

  it('активная дорожка студии при открытии не затирает сохранённую доску', async () => {
    const { board, stem } = usePedalBoardState(() => 5, () => 'drums', PEDAL_STEMS)
    await nextTick()
    expect(stem.value).toBe('drums')
    expect(board.value).toEqual(saved)
    expect(stored().pedals[5]).toEqual({ board: saved, stem: 'drums' })
    expect(stored().other).toBe(1)
  })

  it('правка доски сохраняется на свой трек, чужие треки не трогаются', async () => {
    const { board } = usePedalBoardState(() => 7, () => null, PEDAL_STEMS)
    expect(board.value).toEqual([])
    board.value = [{ chain: 'chorus', params: {}, off: false }]
    await nextTick()
    expect(stored().pedals[7].board).toEqual(board.value)
    expect(stored().pedals[5].board).toEqual(saved)
  })

  it('активная дорожка меняет дорожку доски только если педали на неё ставятся', async () => {
    const active = ref(null)
    const { stem } = usePedalBoardState(() => 5, () => active.value, PEDAL_STEMS)
    active.value = 'kick'           // часть барабанов — не дорожка педалей
    await nextTick()
    expect(stem.value).toBe('bass')
    active.value = ''               // весь трек
    await nextTick()
    expect(stem.value).toBe('')
  })

  it('битое состояние — пустая доска, без исключения', () => {
    globalThis.localStorage = new MemStorage({ [KEY]: '{не json' })
    const { board, stem } = usePedalBoardState(() => 5, () => null, PEDAL_STEMS)
    expect(board.value).toEqual([])
    expect(stem.value).toBe('guitar')
  })
})

describe('PEDAL_STEMS — дорожки доски', () => {
  it('есть «барабаны» и весь трек', () => {
    expect(PEDAL_STEMS).toContain('drums')
    expect(PEDAL_STEMS).toContain('')
  })
})
