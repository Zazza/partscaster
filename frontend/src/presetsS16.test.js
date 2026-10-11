// ТК156–ТК158 (127а–128к, ревью s16): редактор пресета (PresetEditor) и библиотека (PresetLibrary).
// Поведение — скриптом компонента (mountSetup: setup в живом экземпляре, api/демо/плеер/подтверждение подменены),
// раскладка кнопок — деревом разбора шаблона, их :disabled — вычислением выражения шаблона на состоянии компонента.
//   127а — своя занятость save/apply (action): повтор во время запроса не дублирует заявку, кнопки отключены;
//   127б — «Сохранить как свой» не создаёт пресет на сервере до «Сохранить»;
//   127в — закрытие (× и «Отмена») с правками — через подтверждение, без правок — сразу;
//   128а — «▶ на демо» — занятость на каждый пресет;
//   128в — закрытое окно не применяет пресет к доготовившемуся демо;
//   128г — смена своего эталона сбрасывает его дорожки;
//   128д — применение к демо — по семье на момент нажатия;
//   128е — превью мастера — presetMasterChain (то же, что применит Go) по цели громкости;
//   128ж — открытие редактора не создаёт демо;
//   128и — «стало», «было», мастер не играют, если эталон сменили, пока его готовили;
//   128к — превью мастера доступно при пустой цепочке с целью громкости.
import { describe, expect, test, vi, beforeEach } from 'vitest'
import { isRef, nextTick, ref } from 'vue'
import { templateAst, findElements, dir, mountSetup } from './testkit/sfc.js'
import { presetMasterChain } from './presetEdit.js'

vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
const m = vi.hoisted(() => ({ api: {}, demo: {}, confirm: [], before: [] }))
vi.mock('./api.js', () => ({ api: m.api }))
vi.mock('./composables/usePresetDemo.js', () => ({ usePresetDemo: () => m.demo }))
vi.mock('./composables/useConfirm.js', () => ({ useConfirm: () => ({ askConfirm: (...a) => m.confirm.push(a) }) }))
vi.mock('./composables/useWindowPlay.js', () => ({ useWindowPlay: () => ({ play: (w) => m.before.push(w) }) }))
vi.mock('./composables/usePlayer.js', () => ({
  usePlayer: () => ({ toggleArtifact: async (_k, _l, fn) => fn(), playBtn: () => '▶' }),
}))
const { default: PresetEditor } = await import('./components/PresetEditor.vue')
const { default: PresetLibrary } = await import('./components/PresetLibrary.vue')

// обещание, которое тест выполняет сам
function deferred() {
  let resolve
  const promise = new Promise((r) => { resolve = r })
  return { promise, resolve }
}
const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve() }

beforeEach(() => {
  for (const k of Object.keys(m.api)) delete m.api[k]
  Object.assign(m.api, {
    fxAssets: vi.fn(async () => ({ amps: [], irs: [], kits: [] })),
    workerConfig: vi.fn(async () => ({ fx_preview: true })),
    jobs: vi.fn(async () => []),
    soundPresets: vi.fn(async () => []),
    soundPresetCreate: vi.fn(async (body) => ({ id: 11, name: body.name })),
    soundPresetUpdate: vi.fn(async (id, body) => ({ id, name: body.name })),
    applySoundPresetWith: vi.fn(async () => 500),
    applyFx: vi.fn(async () => ({ file: 'p.flac', duration_sec: 15 })),
    playFile: vi.fn(async () => {}),
  })
  m.demo = {
    demos: ref({ 'Рок': { status: 'ready', jobId: 77, file: 'a.flac', durationSec: 60 },
      'Электроника': { status: 'ready', jobId: 88, file: 'b.flac', durationSec: 60 } }),
    ensure: vi.fn(async () => {}),
    ensureStems: vi.fn(async () => {}),
    waitReady: vi.fn(async () => 'ready'),
    available: () => new Set(['bass', 'drums', 'other', 'vocals']),
    refresh: vi.fn(),
    reset: vi.fn(),
  }
  m.confirm.length = 0
  m.before.length = 0
})

const editor = (preset = null) => mountSetup(PresetEditor, { preset, presets: [] })
// выражение шаблона на состоянии компонента: ref — по значению, как в шаблоне
function evalIn(expr, b, extra = {}) {
  const scope = new Proxy({}, {
    has: () => true,
    get: (_, k) => {
      if (k === Symbol.unscopables) return undefined
      const v = k in extra ? extra[k] : b[k]
      return isRef(v) ? v.value : v
    },
  })
  return new Function('scope', `with (scope) { return (${expr}) }`)(scope)
}
const buttonsBy = (rel, click) => findElements(templateAst(rel), (n) => n.tag === 'button' && dir(n, 'on', 'click') === click)
const EDITOR = 'components/PresetEditor.vue'
const LIBRARY = 'components/PresetLibrary.vue'

describe('127а: своя занятость save/apply', () => {
  test('повторное «Сохранить» во время запроса не создаёт второй пресет; «Применить» в это время тоже ждёт', async () => {
    const { b } = await editor()
    const d = deferred()
    m.api.soundPresetCreate = vi.fn(() => d.promise)
    b.model.value.name = 'Мой'
    const first = b.save()
    expect(b.action.value).toBe('save')
    expect(b.canSave.value).toBe(false)
    await b.save()
    await b.applyToRef()
    d.resolve({ id: 11, name: 'Мой' })
    await first
    expect(m.api.soundPresetCreate).toHaveBeenCalledTimes(1)
    expect(m.api.applySoundPresetWith).not.toHaveBeenCalled()
    expect(b.action.value).toBe('')
    // сохранённый новый — дальше обновление, а не второй create
    await b.save()
    expect(m.api.soundPresetUpdate).toHaveBeenCalledTimes(1)
  })

  test('повторное «Применить» во время запроса — одна заявка', async () => {
    const { b } = await editor()
    const d = deferred()
    m.api.applySoundPresetWith = vi.fn(() => d.promise)
    const first = b.applyToRef()
    await flush()
    await b.applyToRef()
    d.resolve(501)
    await first
    expect(m.api.applySoundPresetWith).toHaveBeenCalledTimes(1)
    expect(m.api.applySoundPresetWith.mock.calls[0][0]).toBe(77)
  })

  test('кнопки «Сохранить» и «Применить» отключены, пока идёт любое из двух', async () => {
    const { b } = await editor()
    b.model.value.name = 'Мой'
    const btns = [...buttonsBy(EDITOR, 'save'), ...buttonsBy(EDITOR, 'applyToRef')]
    expect(btns).toHaveLength(2)
    for (const a of ['', 'save', 'apply']) {
      b.action.value = a
      for (const { node } of btns) {
        expect(!!evalIn(dir(node, 'bind', 'disabled'), b), `${dir(node, 'on', 'click')} при action='${a}'`).toBe(a !== '')
      }
    }
  })
})

describe('128в, 128д: применение к демо', () => {
  test('окно закрыли, пока демо доготавливалось, — пресет не применяется', async () => {
    const { b, unmount } = await editor()
    const d = deferred()
    // демо доготовилось ровно к закрытию: ожидание вернуло 'ready', но окно уже закрыто
    m.demo.waitReady = vi.fn(() => d.promise.then(() => 'ready'))
    const run = b.applyToRef()
    await flush()
    unmount()
    d.resolve()
    await run
    expect(m.api.applySoundPresetWith).not.toHaveBeenCalled()
  })

  test('семья демо — та, что была при нажатии, даже если эталон переключили во время ожидания', async () => {
    const { b } = await editor()
    const d = deferred()
    m.demo.ensure = vi.fn(() => d.promise)
    b.refFamily.value = 'Рок'
    const run = b.applyToRef()
    b.refFamily.value = 'Электроника'
    d.resolve()
    await run
    expect(m.demo.waitReady.mock.calls[0][0]).toBe('Рок')
    expect(m.api.applySoundPresetWith.mock.calls[0][0]).toBe(77)
  })
})

describe('127в: закрытие с несохранёнными правками', () => {
  test('без правок — закрывается сразу', async () => {
    const { b, emitted } = await editor()
    b.close()
    expect(emitted).toEqual([['close']])
    expect(m.confirm).toHaveLength(0)
  })

  test('с правками — сначала подтверждение, закрытие — по согласию', async () => {
    const { b, emitted } = await editor()
    b.model.value.name = 'Правка'
    b.close()
    expect(emitted).toEqual([])
    expect(m.confirm).toHaveLength(1)
    m.confirm[0][2]()
    expect(emitted).toEqual([['close']])
  })

  test('× и «Отмена» в шаблоне закрывают через close', () => {
    const closers = buttonsBy(EDITOR, 'close')
    expect(closers.some(({ node }) => dir(node, 'bind', 'title') === "t('common.close')"), '×').toBe(true)
    expect(closers.some(({ node }) => (node.children || []).some((c) => c.type === 5 && /common\.cancel/.test(c.content.content))), '«Отмена»').toBe(true)
  })
})

describe('128ж, 128г: эталон', () => {
  test('открытие редактора не создаёт демо', async () => {
    await editor()
    await flush()
    expect(m.demo.ensure).not.toHaveBeenCalled()
  })

  test('смена своего эталона сбрасывает его дорожки', async () => {
    const { b } = await editor()
    b.ownStems.value = ['bass']
    b.ownJobId.value = 5
    await nextTick()
    expect(b.ownStems.value).toBeNull()
  })
})

describe('128е, 128и, 128к: прослушивание', () => {
  test('превью мастера при пустой цепочке и цели −14: кнопка доступна, цепочка — presetMasterChain', async () => {
    const { b } = await editor()
    b.model.value.targetLufs = -14
    const [btn] = buttonsBy(EDITOR, 'playMasterAfter')
    expect(btn).toBeTruthy()
    expect(!!evalIn(dir(btn.node, 'bind', 'disabled'), b)).toBe(false)
    b.playMasterAfter()
    await flush()
    expect(m.api.applyFx).toHaveBeenCalledTimes(1)
    const [jobId, req] = m.api.applyFx.mock.calls[0]
    expect(jobId).toBe(77)
    expect(req).toMatchObject({ source: 'mix', preview: true, chain: presetMasterChain([], -14) })
  })

  test('ни цепочки, ни цели — кнопка мастера отключена', async () => {
    const { b } = await editor()
    b.model.value.targetLufs = null
    const [btn] = buttonsBy(EDITOR, 'playMasterAfter')
    expect(!!evalIn(dir(btn.node, 'bind', 'disabled'), b)).toBe(true)
  })

  const switchDuring = async (start) => {
    const { b } = await editor()
    b.model.value.targetLufs = -14
    const d = deferred()
    m.demo.ensure = vi.fn(() => d.promise)
    b.refFamily.value = 'Рок'
    const run = start(b)
    b.refFamily.value = 'Электроника'
    d.resolve()
    await run
    await flush()
  }

  test('«стало»: эталон сменили, пока готовили, — не играет', async () => {
    await switchDuring((b) => b.playAfter([{ type: 'comp' }], ['bass'], 'k', 'l'))
    expect(m.api.applyFx).not.toHaveBeenCalled()
  })

  test('«было»: эталон сменили, пока готовили, — не играет', async () => {
    await switchDuring((b) => b.playBefore())
    expect(m.before).toHaveLength(0)
  })

  test('мастер: эталон сменили, пока готовили, — не играет', async () => {
    await switchDuring((b) => b.playMasterAfter())
    expect(m.api.applyFx).not.toHaveBeenCalled()
  })

  test('эталон не меняли — «стало» и «было» играют', async () => {
    const { b } = await editor()
    await b.playAfter([{ type: 'comp' }], ['bass'], 'k', 'l')
    b.playBefore()
    await flush()
    expect(m.api.applyFx).toHaveBeenCalledTimes(1)
    expect(m.before).toHaveLength(1)
  })
})

describe('PresetLibrary (127б, 128а)', () => {
  const lib = () => mountSetup(PresetLibrary, {})

  test('«Сохранить как свой» открывает копию в редакторе, на сервере ничего не создаёт', async () => {
    const { b } = await lib()
    const p = { id: 3, name: 'Пост-панк', builtin: true, specs: [] }
    b.makeCopy(p)
    expect(b.editorPreset.value).toEqual(p)
    expect(m.api.soundPresetCreate).not.toHaveBeenCalled()
  })

  test('«▶ на демо»: повтор того же пресета во время применения — без дубля, другой пресет — параллельно', async () => {
    const { b } = await lib()
    const d = deferred()
    m.api.applySoundPresetWith = vi.fn(() => d.promise)
    const a = { id: 1, name: 'А', family: 'Рок' }
    const c = { id: 2, name: 'Б', family: 'Рок' }
    const runs = [b.playOnDemo(a), b.playOnDemo(a), b.playOnDemo(c)]
    await flush()
    expect(m.api.applySoundPresetWith).toHaveBeenCalledTimes(2)
    expect([...b.demoBusy.value].sort()).toEqual([1, 2])
    // кнопка ▶ отключена по своему пресету
    const [btn] = buttonsBy(LIBRARY, 'playOnDemo(p)')
    expect(btn).toBeTruthy()
    expect(!!evalIn(dir(btn.node, 'bind', 'disabled'), b, { p: a })).toBe(true)
    expect(!!evalIn(dir(btn.node, 'bind', 'disabled'), b, { p: { id: 9 } })).toBe(false)
    d.resolve(600)
    await Promise.all(runs)
    expect(b.demoBusy.value.size).toBe(0)
  })
})
