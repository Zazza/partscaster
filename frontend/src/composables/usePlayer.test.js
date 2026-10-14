// Тесты «играть выделенное» (usePlayer.togglePlay) по контракту: верхний ▶
// при активном выделении гоняет кусок [from, to] артефакта волны — один прогон:
// playFile + перемотка на from (ползунок и тайминг шапки сразу на месте),
// пауза внутри куска — продолжение с места паузы, конец/стоп — новый прогон.
// Без выделения ▶ — обычная пауза/продолжение загруженного.
// Модуль — синглтон с setInterval: каждый тест импортирует его заново.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { unref } from 'vue'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

// node-окружение vitest без DOM: минимальный localStorage (том плеера)
class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

async function flush() {
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
}

const RANGE = { key: 'w15:main', jobId: 15, file: 'audio.flac', dur: 180, from: 160, to: 168 }

async function freshPlayer(state) {
  vi.resetModules()
  const mod = await import('./usePlayer.js')
  const p = mod.usePlayer()
  p.playerState.value = { playing: false, position_sec: 0, duration_sec: 0, job_id: 0, ...state }
  return p
}

describe('▶ шапки: играть выделенное (togglePlay)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('localStorage', new MemStorage())
    apiMock.playFile = vi.fn().mockResolvedValue({})
    apiMock.seekAudio = vi.fn().mockResolvedValue({})
    apiMock.toggleAudio = vi.fn().mockResolvedValue({})
    apiMock.stopAudio = vi.fn().mockResolvedValue({})
    apiMock.audioState = vi.fn().mockResolvedValue({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 })
  })
  afterEach(() => vi.useRealTimers())

  it('без выделения — обычная пауза загруженного', async () => {
    const p = await freshPlayer({ playing: true, job_id: 15, position_sec: 10, duration_sec: 180 })
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalledTimes(1)
    expect(apiMock.playFile).not.toHaveBeenCalled()
  })

  it('с выделением и без loaded — прогон куска: playFile, ползунок на from, стоп в конце', async () => {
    const p = await freshPlayer()
    p.setPlayRange(RANGE)
    await p.togglePlay()
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(15, 'audio.flac', 180)
    expect(apiMock.seekAudio).toHaveBeenCalledWith(160)
    expect(unref(p.seekPos)).toBe(160)              // ползунок сразу на начале куска
    expect(unref(p.rangeUntil)).toEqual({ key: 'w15:main', to: 168 })
    expect(unref(p.nowPlayingKey)).toBe('w15:main')
  })

  it('пауза внутри куска — продолжение с места паузы, без нового playFile', async () => {
    const p = await freshPlayer({ playing: false, job_id: 15, position_sec: 163, duration_sec: 180 })
    p.setPlayRange(RANGE)
    p.nowPlayingKey.value = 'w15:main'
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalledTimes(1)
    expect(apiMock.playFile).not.toHaveBeenCalled()
  })

  it('играет кусок — повторный ▶ это пауза', async () => {
    const p = await freshPlayer({ playing: true, job_id: 15, position_sec: 162, duration_sec: 180 })
    p.setPlayRange(RANGE)
    p.nowPlayingKey.value = 'w15:main'
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalledTimes(1)
    expect(apiMock.playFile).not.toHaveBeenCalled()
  })

  it('позиция за концом куска — новый прогон с начала', async () => {
    const p = await freshPlayer({ playing: false, job_id: 15, position_sec: 170, duration_sec: 180 })
    p.setPlayRange(RANGE)
    await p.togglePlay()
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledTimes(1)
    expect(apiMock.seekAudio).toHaveBeenCalledWith(160)
  })

  it('снятие выделения очищает и действующий прогон', () => {
    // отдельно от freshPlayer: модуль уже импортирован предыдущими тестами
    return freshPlayer().then((p) => {
      p.setPlayRange(RANGE)
      p.playerState.value = { ...p.playerState.value, playing: false }
      p.rangeUntil.value = { key: 'w15:main', to: 168 }
      p.setPlayRange(null)
      expect(unref(p.playRange)).toBeNull()
      expect(unref(p.rangeUntil)).toBeNull()
    })
  })
})

// playbackEnded(state) по контракту: true только когда трек доиграл до конца —
// не играет, длительность > 0 и позиция не дальше полсекунды от конца
// (допуск на округление таймера). Остальное — false.
describe('playbackEnded: трек доиграл до конца', () => {
  let playbackEnded
  beforeEach(async () => {
    vi.stubGlobal('localStorage', new MemStorage())
    vi.resetModules()
    ;({ playbackEnded } = await import('./usePlayer.js'))
  })
  afterEach(() => vi.unstubAllGlobals())

  const st = (s) => ({ playing: false, position_sec: 0, duration_sec: 180, job_id: 15, ...s })

  it('остановлен ровно на конце — true', () => {
    expect(playbackEnded(st({ position_sec: 180 }))).toBe(true)
  })

  it('играет — false, даже на последней секунде и на самом конце', () => {
    expect(playbackEnded(st({ playing: true, position_sec: 179.8 }))).toBe(false)
    expect(playbackEnded(st({ playing: true, position_sec: 180 }))).toBe(false)
  })

  it('пауза в середине — false', () => {
    expect(playbackEnded(st({ position_sec: 90 }))).toBe(false)
  })

  it('ничего не загружено: duration 0 или нет — false', () => {
    expect(playbackEnded(st({ duration_sec: 0, position_sec: 0 }))).toBe(false)
    const noDur = st({ position_sec: 0 })
    delete noDur.duration_sec
    expect(playbackEnded(noDur)).toBe(false)
  })

  it('state null/undefined — false', () => {
    expect(playbackEnded(null)).toBe(false)
    expect(playbackEnded(undefined)).toBe(false)
  })

  it('граница допуска: duration-0.5 — true, duration-0.6 — false', () => {
    expect(playbackEnded(st({ position_sec: 179.5 }))).toBe(true)
    expect(playbackEnded(st({ position_sec: 179.4 }))).toBe(false)
  })

  it('позиция больше длительности — true', () => {
    expect(playbackEnded(st({ position_sec: 181 }))).toBe(true)
  })
})

describe('▶ панели: память последнего артефакта (togglePlay без выделения)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('localStorage', new MemStorage())
    apiMock.playFile = vi.fn().mockResolvedValue({})
    apiMock.seekAudio = vi.fn().mockResolvedValue({})
    apiMock.toggleAudio = vi.fn().mockResolvedValue({})
    apiMock.stopAudio = vi.fn().mockResolvedValue({})
    apiMock.audioState = vi.fn().mockResolvedValue({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 })
  })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  it('после «стопа» плей панели возвращает последний запущенный артефакт, а не что попало', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.markNow('v882:overdub-inst-0.flac', '#882 превью', 882, 'overdub-inst-0.flac', 180)
    await p.togglePlay()
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'overdub-inst-0.flac', 180)
    expect(apiMock.toggleAudio).not.toHaveBeenCalled()
    expect(p.nowPlayingKey.value).toBe('v882:overdub-inst-0.flac')
  })

  it('играет или на паузе в середине — обычная пауза/продолжение, без перезапуска', async () => {
    const p = await freshPlayer({ job_id: 7, playing: true, position_sec: 42, duration_sec: 180 })
    p.markNow('m7', '#7', 7, 'audio.flac', 180)
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalled()
    expect(apiMock.playFile).not.toHaveBeenCalled()
  })

  it('выделение в студии гоняет кусок, пока оно и есть последний артефакт', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setPlayRange(RANGE)
    p.markNow(RANGE.key, 'кусок', RANGE.jobId, RANGE.file, RANGE.dur)
    await p.togglePlay()
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(RANGE.jobId, RANGE.file, RANGE.dur)
    expect(apiMock.seekAudio).toHaveBeenCalledWith(RANGE.from)
  })

  it('после «послушать превью» плей панели не уводит на выделение волны', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setPlayRange(RANGE)
    p.markNow('v882:overdub-inst-0.flac', '#882 превью', 882, 'overdub-inst-0.flac', 180)
    await p.togglePlay()
    await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'overdub-inst-0.flac', 180)
    expect(apiMock.seekAudio).not.toHaveBeenCalled()   // не прогон куска выделения
  })

  it('нет памяти — прежнее поведение: пауза/продолжение загруженного', async () => {
    const p = await freshPlayer({ job_id: 7, playing: false, position_sec: 42, duration_sec: 180 })
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalled()
  })
})

describe('▶ панели: студийный контекст (togglePlay в студии)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('localStorage', new MemStorage())
    apiMock.playFile = vi.fn().mockResolvedValue({})
    apiMock.seekAudio = vi.fn().mockResolvedValue({})
    apiMock.toggleAudio = vi.fn().mockResolvedValue({})
    apiMock.stopAudio = vi.fn().mockResolvedValue({})
    apiMock.audioState = vi.fn().mockResolvedValue({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 })
  })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals() })

  const CTX = (over = {}) => ({ jobId: 882, file: 'audio.flac', mainFile: 'audio.flac', mixFile: null, dur: 200, label: '#882 трек', ...over })

  it('зашёл в студию, ничего не слушал — плей играет сам трек', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setStudioCtx(CTX())
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'audio.flac', 200)
  })

  it('в студии есть правки — плей играет свежий микс (последнюю правку)', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setStudioCtx(CTX({ file: 'overdub-inst-0.flac', mixFile: 'overdub-inst-0.flac' }))
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'overdub-inst-0.flac', 200)
  })

  it('слушали в этом треке превью — плей возвращает превью, а не микс', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setStudioCtx(CTX({ file: 'overdub-inst-0.flac', mixFile: 'overdub-inst-0.flac' }))
    p.markNow('v882:overdub-inst-0.flac', '#882 превью', 882, 'overdub-inst-0.flac', 200)
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'overdub-inst-0.flac', 200)
    expect(apiMock.playFile).toHaveBeenCalledTimes(1)
  })

  it('слушали другой трек в списке, зашли в студию — плей играет трек студии', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.markNow('m7', '#7', 7, 'audio.flac', 180)
    p.setStudioCtx(CTX())
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'audio.flac', 200)
  })

  it('устаревшая память «просто оригинал» при появившихся правках — играет микс', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.markNow('w882:main', '#882', 882, 'audio.flac', 200)
    p.setStudioCtx(CTX({ file: 'overdub-inst-0.flac', mixFile: 'overdub-inst-0.flac' }))
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(882, 'overdub-inst-0.flac', 200)
  })

  it('загружен трек студии на паузе в середине — плей продолжает, не перезапускает', async () => {
    const p = await freshPlayer({ job_id: 882, playing: false, position_sec: 60, duration_sec: 200 })
    p.setStudioCtx(CTX())
    await p.togglePlay()
    expect(apiMock.toggleAudio).toHaveBeenCalled()
    expect(apiMock.playFile).not.toHaveBeenCalled()
  })

  it('студию закрыли (контекст снят) — прежняя логика списка', async () => {
    const p = await freshPlayer({ job_id: 0, playing: false, position_sec: 0 })
    p.setStudioCtx(CTX())
    p.setStudioCtx(null)
    p.markNow('m7', '#7', 7, 'audio.flac', 180)
    await p.togglePlay(); await flush()
    expect(apiMock.playFile).toHaveBeenCalledWith(7, 'audio.flac', 180)
  })
})
