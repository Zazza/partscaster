// engineRun.js — логика «применить цепочку движка» (EngineBox) и «докачать наборы» (EngineBox, InstrumentsPage)
// вне компонентов.
//
// Контракт:
// - ensureKits(api, chain, kits): chain — цепочка для воркера ({type, ...параметры}, как у
//   missingKits), kits — список наборов воркера ([{name: 'набор/часть', samples}], как в
//   fxAssets().kits). Недостающие наборы (по kit и kit_open блоков sampler/bass) ставятся
//   api.installFxKit(имя набора) по одному (следующий — после окончания предыдущего), в
//   порядке цепочки, каждый один раз; уже стоящие не трогаются. Ошибка установки →
//   отклонённый промис с текстом ошибки.
// - engineRecord(snap) (internal-studio-instant-apply, условия 1–2): «Применить» цепочки движка только пишет
//   запись — без превью, установки наборов и запросов к воркеру; api функция не получает. Синхронная.
//   snap — снимок EngineBox {src, chain, from, to, label}; ответ — запись для useInserts.addStemEngine:
//   {stem: src, chain: глубокая копия snap.chain, from, to, label}; окно как есть (to ≤ from — «до конца»);
//   запись не делит объектов с snap.chain.
import { describe, it, expect } from 'vitest'
import { ensureKits, engineRecord } from './engineRun.js'

// Фейк api: асинхронные методы, журнал вызовов, хуки «во время ожидания».
// hooks[method] вызывается ПОСЛЕ того, как ensureKits вызвал метод, но до
// того, как его промис разрешился, — это и есть «изменение во время await».
function fakeApi({
  config = { fx_preview: true },
  kits = [],
  applyResp = { file: 'preview-fx-0a1b2c3d.flac', duration_sec: 10, clipped: false },
  installErr = {},
  hooks = {},
} = {}) {
  const log = []
  const tick = () => new Promise((r) => setTimeout(r, 0))
  const api = {
    async workerConfig() {
      log.push(['workerConfig'])
      hooks.workerConfig?.()
      await tick()
      return config
    },
    async fxAssets() {
      log.push(['fxAssets'])
      hooks.fxAssets?.()
      await tick()
      return { amps: [], irs: [], kits: kits.map((k) => ({ ...k })) }
    },
    async installFxKit(name) {
      log.push(['installFxKit:start', name])
      hooks.installFxKit?.(name)
      await tick()
      if (installErr[name]) {
        log.push(['installFxKit:fail', name])
        throw new Error(installErr[name])
      }
      log.push(['installFxKit:end', name])
      return { name, downloaded: true }
    },
    async applyFx(id, req) {
      // глубокая копия: позднее изменение объектов не должно «переписать» журнал
      log.push(['applyFx', id, JSON.parse(JSON.stringify(req))])
      await tick()
      return applyResp
    },
  }
  const calls = (m) => log.filter((e) => e[0] === m)
  const installs = () => log.filter((e) => e[0] === 'installFxKit:start').map((e) => e[1])
  return { api, log, calls, installs }
}

const kit = (name) => ({ name, samples: 3 })

describe('ensureKits: недостающие наборы', () => {
  it('всё стоит → ничего не ставит', async () => {
    const f = fakeApi()
    const chain = [{ type: 'sampler', kit: 'osdk/kick' }, { type: 'gain', gain_db: 0 }]
    await ensureKits(f.api, chain, [kit('osdk/kick')])
    expect(f.installs()).toEqual([])
  })

  it('цепочка без наборов → ничего не ставит', async () => {
    const f = fakeApi()
    await ensureKits(f.api, [{ type: 'eq' }, { type: 'reverb' }], [])
    expect(f.installs()).toEqual([])
  })

  it('kit есть, kit_open нет → ставится набор kit_open', async () => {
    const f = fakeApi()
    const chain = [{ type: 'sampler', kit: 'osdk/hh-closed', kit_open: 'osdk/hh-half', choke: 1 }]
    await ensureKits(f.api, chain, [kit('osdk/hh-closed')])
    expect(f.installs()).toEqual(['osdk'])
  })

  it('kit нет → ставится набор kit (и для bass)', async () => {
    const f = fakeApi()
    await ensureKits(f.api, [{ type: 'bass', kit: 'growlybass/bass' }], [kit('osdk/kick')])
    expect(f.installs()).toEqual(['growlybass'])
  })

  it('порядок как в цепочке, каждый набор один раз, стоящие не трогает', async () => {
    const f = fakeApi()
    const chain = [
      { type: 'bass', kit: 'growlybass/bass' },
      { type: 'sampler', kit: 'osdk/kick' },                                // стоит
      { type: 'sampler', kit: 'mykit/hh', kit_open: 'other/open' },
      { type: 'sampler', kit: 'osdk/snare', kit_open: 'growlybass/bass' },  // osdk/snare нет, growlybass — уже
    ]
    await ensureKits(f.api, chain, [kit('osdk/kick')])
    expect(f.installs()).toEqual(['growlybass', 'mykit', 'other', 'osdk'])
  })

  it('пустой список наборов воркера → ставит все наборы цепочки', async () => {
    const f = fakeApi()
    const chain = [{ type: 'sampler', kit: 'osdk/kick' }, { type: 'bass', kit: 'growlybass/bass' }]
    await ensureKits(f.api, chain, [])
    expect(f.installs()).toEqual(['osdk', 'growlybass'])
  })

  it('по одному: следующая установка начинается после окончания предыдущей', async () => {
    const f = fakeApi()
    const chain = [{ type: 'sampler', kit: 'a/x' }, { type: 'sampler', kit: 'b/y' }, { type: 'bass', kit: 'c/z' }]
    await ensureKits(f.api, chain, [])
    const seq = f.log.filter((e) => e[0].startsWith('installFxKit')).map((e) => `${e[0]}:${e[1]}`)
    expect(seq).toEqual([
      'installFxKit:start:a', 'installFxKit:end:a',
      'installFxKit:start:b', 'installFxKit:end:b',
      'installFxKit:start:c', 'installFxKit:end:c',
    ])
  })

  it('ошибка установки → отклонённый промис с текстом, дальше не ставит', async () => {
    const f = fakeApi({ installErr: { b: 'набор b: сеть недоступна' } })
    const chain = [{ type: 'sampler', kit: 'a/x' }, { type: 'sampler', kit: 'b/y' }, { type: 'sampler', kit: 'c/z' }]
    await expect(ensureKits(f.api, chain, [])).rejects.toThrow(/сеть недоступна/)
    expect(f.installs()).toEqual(['a', 'b'])
  })

  it('вход не меняет', async () => {
    const f = fakeApi()
    const chain = Object.freeze([Object.freeze({ type: 'sampler', kit: 'osdk/kick', kit_open: 'osdk/hh-half' })])
    const kits = Object.freeze([Object.freeze(kit('osdk/kick'))])
    await ensureKits(f.api, chain, kits)
    expect(f.installs()).toEqual(['osdk'])
  })
})

const CHAIN = () => [
  { type: 'amp', model: 'JCM2000.nam', input_db: 0, output_db: -3 },
  { type: 'eq', highpass_hz: 80, lowpass_hz: 0, bands: [{ freq_hz: 1000, gain_db: -3, q: 1 }] },
]
const SNAP = (over = {}) => ({
  jobId: 466, dur: 180, src: 'other', from: 20, to: 35, chain: CHAIN(), label: 'Гитара: JCM2000', ...over,
})

describe('engineRecord: запись без запросов к воркеру', () => {
  it('запись: stem = src, chain = цепочка снимка, окно и подпись как есть', () => {
    const rec = engineRecord(SNAP())
    expect(rec).toMatchObject({ stem: 'other', from: 20, to: 35, label: 'Гитара: JCM2000' })
    expect(rec.chain).toEqual(CHAIN())
  })

  it('синхронная: возвращает запись, а не промис', () => {
    const rec = engineRecord(SNAP())
    expect(typeof rec?.then).not.toBe('function')
  })

  it('to ≤ from («до конца») хранится как есть', () => {
    expect(engineRecord(SNAP({ from: 40, to: 0 }))).toMatchObject({ from: 40, to: 0 })
    expect(engineRecord(SNAP({ from: 40, to: 30 }))).toMatchObject({ from: 40, to: 30 })
  })

  it('from = 0, to = 0 — весь трек', () => {
    expect(engineRecord(SNAP({ from: 0, to: 0 }))).toMatchObject({ from: 0, to: 0 })
  })

  it('цепочка с наборами — запись без установки (api не нужен)', () => {
    const chain = [{ type: 'sampler', kit: 'osdk/kick', kit_open: 'osdk/hh-open' }, { type: 'bass', kit: 'growlybass/bass' }]
    expect(engineRecord(SNAP({ src: 'kick', chain })).chain).toEqual(chain)
  })

  it('запись не делит объекты с исходной цепочкой: правка источника после вызова запись не меняет', () => {
    const snap = SNAP()
    const rec = engineRecord(snap)
    snap.chain[0].input_db = 12
    snap.chain[1].bands[0].gain_db = 6
    snap.chain.push({ type: 'reverb' })
    expect(rec.chain).toEqual(CHAIN())
    rec.chain[0].output_db = 0
    expect(snap.chain[0].output_db).toBe(-3)
  })

  it('снимок не меняется', () => {
    const snap = SNAP()
    engineRecord(snap)
    expect(snap).toEqual(SNAP())
  })
})
