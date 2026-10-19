// frontend/src/engine.js — чистые функции блока «Модель генерации» и метки модели у трека
// (карточка internal-cpp-only: генерация только на yue2.cpp, переключателя движка нет).
//
// Тест-кейсы (ТК → условие карточки):
//   ТК22 (усл. 8)  currentModel(config) — config.cpp_model; конфиг не загружен или без модели — '';
//   ТК23 (усл. 8а) modelBadge(job, config): '' пока конфиг не загружен и когда модель трека
//                  (job.engine "cpp-q8" → q8) равна текущей; иначе метка модели трека: «BF16», «Q8»…;
//                  старые треки (engine "" / "torch" / без поля) — «PyTorch»;
//   ТК24 (усл. 8а) retryWarning(job, config): по тому же правилу — '' или непустой текст
//                  (старые треки — всегда, когда конфиг загружен);
//   ТК25 (усл. 8, 3) modelOptions(config) → [{value, label, installed, recommended}] в порядке
//                  bf16, q8, q6, q5 (только те, что есть в cpp_models); recommended — у
//                  cpp_recommended.model; label — имя модели и размер в ГБ (один знак);
//   ТК26 (усл. 2, 8) modelConfig(model, lowVram) → {cpp_model, cpp_low_vram} без engine;
//                  старых экспортов engineConfig, currentEngineLabel, engineLabel нет;
//   ТК27 (усл. 8)  словари ru/en: про генерацию нет «эксперимент…»/«experimental» рядом с yue2.cpp,
//                  PyTorch или движком, нет «основной (PyTorch)»/«основной движок»; пометка
//                  «рекомендуется» / «recommended» есть в обоих словарях.
// Тексты, кроме меток моделей, не фиксируются — проверяются непустота и числа.
// Паритет ключей ru/en проверяет общий i18n.test.js.
import { describe, it, expect, vi } from 'vitest'
// engine.js берёт тексты через общий i18n, а он читает язык из localStorage —
// в node его нет, подставляем пустое хранилище (как activeTrack.test.js)
vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
import * as engine from './engine.js'
import ru from './i18n/ru.js'
import en from './i18n/en.js'
import { currentModel, modelBadge, retryWarning, modelOptions, modelConfig } from './engine.js'

const MODELS = {
  // порядок ключей нарочно не тот, что в списке
  q5: { file: 'YuE2-3B-Q5_K_M.gguf', size: 2_420_000_000, installed: false },
  q8: { file: 'YuE2-3B-Q8_0.gguf', size: 3_810_232_064, installed: true },
  bf16: { file: 'YuE2-3B-BF16.gguf', size: 7_130_000_000, installed: false },
  q6: { file: 'YuE2-3B-Q6_K.gguf', size: 2_940_000_000, installed: false },
}
const cfg = (model, extra = {}) => ({
  cpp_model: model,
  cpp_low_vram: false,
  cpp_available: true,
  cpp_models: MODELS,
  cpp_recommended: { model: 'bf16', low_vram: false, vram_mib: 12282 },
  ...extra,
})

describe('currentModel (ТК22)', () => {
  it('модель из настроек воркера', () => {
    expect(currentModel(cfg('q6'))).toBe('q6')
    expect(currentModel(cfg('bf16'))).toBe('bf16')
  })
  it('конфиг не загружен — пусто', () => {
    expect(currentModel(null)).toBe('')
    expect(currentModel(undefined)).toBe('')
  })
  it('в конфиге нет модели — пусто', () => {
    expect(currentModel({})).toBe('')
  })
})

describe('modelBadge (ТК23)', () => {
  it('модель трека та же, что сейчас, — метки нет', () => {
    expect(modelBadge({ engine: 'cpp-q8' }, cfg('q8'))).toBe('')
    expect(modelBadge({ engine: 'cpp-bf16' }, cfg('bf16'))).toBe('')
  })

  it('модель трека другая — метка модели трека', () => {
    expect(modelBadge({ engine: 'cpp-bf16' }, cfg('q8'))).toBe('BF16')
    expect(modelBadge({ engine: 'cpp-q8' }, cfg('bf16'))).toBe('Q8')
    expect(modelBadge({ engine: 'cpp-q6' }, cfg('q8'))).toBe('Q6')
    expect(modelBadge({ engine: 'cpp-q5' }, cfg('q8'))).toBe('Q5')
  })

  it('старые треки PyTorch ("" / "torch" / без поля) — «PyTorch»', () => {
    for (const m of ['q8', 'bf16']) {
      expect(modelBadge({ engine: '' }, cfg(m))).toBe('PyTorch')
      expect(modelBadge({ engine: 'torch' }, cfg(m))).toBe('PyTorch')
      expect(modelBadge({ id: 1 }, cfg(m))).toBe('PyTorch')
    }
  })

  // конфиг не загружен — текущая модель неизвестна, ложной метки нет
  it('конфиг не загружен — метки нет', () => {
    for (const e of ['cpp-q8', 'cpp-bf16', 'torch', '']) {
      expect(modelBadge({ engine: e }, null), `трек ${e || '""'}`).toBe('')
      expect(modelBadge({ engine: e }, undefined), `трек ${e || '""'}`).toBe('')
    }
  })
})

describe('retryWarning (ТК24)', () => {
  it('та же модель — ничего', () => {
    expect(retryWarning({ id: 1, engine: 'cpp-q8' }, cfg('q8'))).toBe('')
    expect(retryWarning({ id: 1, engine: 'cpp-bf16' }, cfg('bf16'))).toBe('')
  })

  // сид между моделями не переносится — другая модель = другое исполнение
  it('другая модель — предупреждение', () => {
    const s = retryWarning({ id: 1, engine: 'cpp-q8' }, cfg('bf16'))
    expect(typeof s).toBe('string')
    expect(s).not.toBe('')
    expect(retryWarning({ id: 1, engine: 'cpp-bf16' }, cfg('q5'))).not.toBe('')
  })

  it('старый трек PyTorch — всегда предупреждение', () => {
    for (const m of ['q8', 'bf16', 'q6', 'q5']) {
      expect(retryWarning({ id: 1, engine: '' }, cfg(m))).not.toBe('')
      expect(retryWarning({ id: 1, engine: 'torch' }, cfg(m))).not.toBe('')
      expect(retryWarning({ id: 1 }, cfg(m))).not.toBe('')
    }
  })

  it('конфиг не загружен — ничего', () => {
    for (const e of ['cpp-q8', 'torch', '']) {
      expect(retryWarning({ id: 1, engine: e }, null)).toBe('')
      expect(retryWarning({ id: 1, engine: e }, undefined)).toBe('')
    }
  })
})

describe('modelOptions (ТК25)', () => {
  it('все четыре модели в порядке bf16, q8, q6, q5', () => {
    expect(modelOptions(cfg('q8')).map((o) => o.value)).toEqual(['bf16', 'q8', 'q6', 'q5'])
  })

  it('признак «скачана» — из installed', () => {
    const by = Object.fromEntries(modelOptions(cfg('q8')).map((o) => [o.value, o]))
    expect(by.q8.installed).toBe(true)
    expect(by.bf16.installed).toBe(false)
    expect(by.q6.installed).toBe(false)
    expect(by.q5.installed).toBe(false)
  })

  it('рекомендуемая помечена — ровно одна, из cpp_recommended', () => {
    const opts = modelOptions(cfg('q8'))
    expect(opts.filter((o) => o.recommended).map((o) => o.value)).toEqual(['bf16'])
    const small = modelOptions(cfg('q8', { cpp_recommended: { model: 'q8', low_vram: true, vram_mib: 8188 } }))
    expect(small.filter((o) => o.recommended).map((o) => o.value)).toEqual(['q8'])
  })

  it('воркер без cpp_recommended — ничего не помечено', () => {
    const opts = modelOptions({ cpp_model: 'q8', cpp_models: MODELS })
    expect(opts.map((o) => o.value)).toEqual(['bf16', 'q8', 'q6', 'q5'])
    expect(opts.some((o) => o.recommended)).toBe(false)
  })

  it('подпись — имя модели и размер в ГБ с одним знаком', () => {
    const by = Object.fromEntries(modelOptions(cfg('q8')).map((o) => [o.value, o]))
    expect(by.bf16.label).toContain('BF16')
    expect(by.bf16.label).toMatch(/(?<![\d.,])7[.,]1(?![\d])/)
    expect(by.q8.label).toContain('Q8')
    expect(by.q8.label).toMatch(/(?<![\d.,])3[.,]8(?![\d])/)
    expect(by.q8.label).not.toContain('3810232064')
  })

  it('подпись скачанной и нескачанной модели различается', () => {
    const one = (installed) =>
      modelOptions({ cpp_models: { q8: { ...MODELS.q8, installed } } }).find((o) => o.value === 'q8').label
    expect(one(true)).not.toBe(one(false))
  })

  it('в cpp_models только часть моделей — только они', () => {
    const opts = modelOptions({ cpp_models: { q6: MODELS.q6, q8: MODELS.q8 } })
    expect(opts.map((o) => o.value)).toEqual(['q8', 'q6'])
  })

  it('конфиг без cpp_models или не загружен — пустой список', () => {
    expect(modelOptions({})).toEqual([])
    expect(modelOptions(null)).toEqual([])
    expect(modelOptions(undefined)).toEqual([])
  })
})

describe('modelConfig — что уходит воркеру при сохранении (ТК26)', () => {
  it('модель и экономный режим, без engine', () => {
    expect(modelConfig('bf16', false)).toEqual({ cpp_model: 'bf16', cpp_low_vram: false })
    expect(modelConfig('q6', true)).toEqual({ cpp_model: 'q6', cpp_low_vram: true })
  })
  it('экономный режим приводится к bool', () => {
    expect(modelConfig('q8', undefined)).toEqual({ cpp_model: 'q8', cpp_low_vram: false })
    expect(modelConfig('q8', 1)).toEqual({ cpp_model: 'q8', cpp_low_vram: true })
  })
})

describe('выбора движка нет (ТК26)', () => {
  it('старых экспортов нет', () => {
    expect(engine.engineConfig).toBeUndefined()
    expect(engine.currentEngineLabel).toBeUndefined()
    expect(engine.engineLabel).toBeUndefined()
  })
})

describe('тексты интерфейса без движка PyTorch (ТК27)', () => {
  const dicts = { ru, en }
  it('нет «экспериментально» про yue2.cpp/PyTorch/движок генерации', () => {
    for (const [lang, d] of Object.entries(dicts)) {
      const bad = Object.entries(d).filter(([, v]) =>
        typeof v === 'string' && /эксперимент|experiment/i.test(v) && /yue2|pytorch|движ|engine/i.test(v))
      expect(bad, lang).toEqual([])
    }
  })
  it('нет «основной (PyTorch)» и «основной движок»', () => {
    for (const [lang, d] of Object.entries(dicts)) {
      const bad = Object.entries(d).filter(([, v]) =>
        typeof v === 'string' && /основн\S* \(pytorch\)|основной движок|main \(pytorch\)|main engine|primary engine/i.test(v))
      expect(bad, lang).toEqual([])
    }
  })
  it('пометка рекомендуемой модели есть в обоих словарях', () => {
    expect(Object.values(ru).some((v) => typeof v === 'string' && /рекомендуется/i.test(v))).toBe(true)
    expect(Object.values(en).some((v) => typeof v === 'string' && /recommended/i.test(v))).toBe(true)
  })
})
