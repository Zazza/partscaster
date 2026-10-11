// Условие 136 (internal-own-track, этап 17): useWindowPlay — «▶ было»: кусок [from, to) файла трека
// встроенным плеером со стопом в конце куска (плеер сам конца не знает).
//   - ▶: файл трека загружается в плеер и перематывается на from; кнопка этого куска — «играет»;
//   - позиция дошла до to — стоп (один раз);
//   - первая позиция может остаться от прошлого проигрывания (за концом куска) — по ней не стопать,
//     следить только после того, как позиция побывала внутри куска;
//   - играет другое (кнопка другого артефакта) — этот кусок плеер не останавливает;
//   - компонент закрыли, пока кусок играет, — стоп (следить за концом больше некому);
//     закрыли без проигрывания — плеер не трогается.
// Плеер — настоящий usePlayer; мок только на границе — api (Wails-биндинги плеера).
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createRenderer, defineComponent } from 'vue'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

// рендерер без DOM: компоненту нужен только жизненный цикл (onBeforeUnmount)
const { createApp } = createRenderer({
  insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
})

const KEY = 'perc-before'
const JOB = 7

async function flush() {
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
}

// свежие модули (usePlayer — синглтон) и смонтированный компонент с useWindowPlay
async function mountWindowPlay() {
  vi.resetModules()
  const { usePlayer } = await import('./usePlayer.js')
  const { useWindowPlay } = await import('./useWindowPlay.js')
  const player = usePlayer()
  let w
  const app = createApp(defineComponent({
    setup() {
      w = useWindowPlay(KEY, () => 'было')
      return () => null
    },
  }))
  app.mount({})
  return { player, play: (r) => w.play(r), unmount: () => app.unmount() }
}

// плеер сообщил позицию (как опрос audioState)
async function at(player, pos) {
  player.playerState.value = { playing: true, position_sec: pos, duration_sec: 180, job_id: JOB }
  await flush()
}

const RANGE = { jobId: JOB, file: 'audio.flac', dur: 180, from: 20, to: 35 }

describe('«▶ было»: кусок со стопом в конце (useWindowPlay)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('localStorage', new MemStorage())
    apiMock.playFile = vi.fn().mockResolvedValue({})
    apiMock.seekAudio = vi.fn().mockResolvedValue({})
    apiMock.stopAudio = vi.fn().mockResolvedValue({})
    apiMock.toggleAudio = vi.fn().mockResolvedValue({})
    apiMock.audioState = vi.fn().mockResolvedValue({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('▶: файл трека в плеер, перемотка на начало куска, кнопка куска — «играет»', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(JOB, 'audio.flac', 180)
    expect(apiMock.seekAudio).toHaveBeenCalledWith(20)
    expect(player.nowPlayingKey.value).toBe(KEY)
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
  })

  it('внутри куска не стопает; позиция дошла до конца — стоп', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 20.2)
    await at(player, 30)
    await at(player, 34.9)
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
    await at(player, 35)
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
    expect(player.nowPlayingKey.value).toBe('')
  })

  it('перескок за конец (опрос редкий) — тоже стоп', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    await at(player, 41)
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
  })

  it('стоп один раз: дальнейшие позиции после конца плеер не трогают', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    await at(player, 36)
    await at(player, 37)
    await at(player, 38)
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
    expect(apiMock.playFile).toHaveBeenCalledTimes(1)   // остановка не перезапускает кусок
  })

  it('позиция от прошлого проигрывания (за концом куска) первой — не стоп', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 120)   // плеер ещё не успел перемотать: старая позиция
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
    expect(player.nowPlayingKey.value).toBe(KEY)
    await at(player, 20.1)  // перемотка дошла
    await at(player, 35.2)
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
  })

  it('играет другой артефакт — этот кусок его не останавливает', async () => {
    const { player, play } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    player.nowPlayingKey.value = 'm7'   // человек нажал ▶ у трека
    await at(player, 50)
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
    expect(player.nowPlayingKey.value).toBe('m7')   // кнопка трека — по-прежнему «играет»
  })

  it('компонент закрыли, пока кусок играет, — стоп', async () => {
    const { player, play, unmount } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    unmount()
    await flush()
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
    expect(player.nowPlayingKey.value).toBe('')
  })

  it('закрыли без проигрывания — плеер не трогается', async () => {
    const { unmount } = await mountWindowPlay()
    unmount()
    await flush()
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
  })

  it('закрыли, когда играет уже другое, — чужое не останавливается', async () => {
    const { player, play, unmount } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    player.nowPlayingKey.value = 'm7'
    unmount()
    await flush()
    expect(apiMock.stopAudio).not.toHaveBeenCalled()
    expect(player.nowPlayingKey.value).toBe('m7')
  })

  it('сбой остановки при закрытии не роняет закрытие', async () => {
    apiMock.stopAudio = vi.fn().mockRejectedValue(new Error('плеер уже остановлен'))
    const { player, play, unmount } = await mountWindowPlay()
    await play(RANGE)
    await at(player, 25)
    expect(() => unmount()).not.toThrow()
    await flush()
    expect(apiMock.stopAudio).toHaveBeenCalledTimes(1)
  })
})
