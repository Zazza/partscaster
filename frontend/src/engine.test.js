// frontend/src/engine.js — чистые функции блока «Движок генерации» и карточки трека:
//   - engineLabel(job): метка движка трека по job.engine ("" у старых = PyTorch; "torch" | "cpp-bf16" |
//     "cpp-q8" | "cpp-q6" | "cpp-q5"): yue2.cpp → «yue2.cpp Q8» (BF16/Q8/Q6/Q5), PyTorch и старые → '' (ничего);
//   - retryWarning(job, currentEngine): currentEngine — действующий движок в виде метки трека
//     ("torch" | "cpp-q8" …); трек другого движка → непустое предупреждение, тот же → ''; старый трек ("") = PyTorch;
//   - modelOptions(config): по config.cpp_models {имя: {file, size, installed}} → [{value, label, installed}]
//     в порядке bf16, q8, q6, q5; label — размер в ГБ (один знак) и признак «скачана».
// Тексты, кроме «yue2.cpp Q8», не фиксируются — проверяются только непустота и числа.
// Паритет ключей ru/en и наличие каждого ключа t('…') в обоих словарях проверяет общий i18n.test.js.
import { describe, it, expect, vi } from 'vitest'
// engine.js берёт тексты через общий i18n, а он читает язык из localStorage —
// в node его нет, подставляем пустое хранилище (как activeTrack.test.js)
vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
import { engineLabel, retryWarning, modelOptions, engineConfig } from './engine.js'

describe('engineLabel', () => {
  it('трек yue2.cpp — «yue2.cpp <МОДЕЛЬ>»', () => {
    expect(engineLabel({ engine: 'cpp-q8' })).toBe('yue2.cpp Q8')
    expect(engineLabel({ engine: 'cpp-bf16' })).toBe('yue2.cpp BF16')
    expect(engineLabel({ engine: 'cpp-q6' })).toBe('yue2.cpp Q6')
    expect(engineLabel({ engine: 'cpp-q5' })).toBe('yue2.cpp Q5')
  })

  it('PyTorch и старые треки — ничего', () => {
    expect(engineLabel({ engine: 'torch' })).toBe('')
    expect(engineLabel({ engine: '' })).toBe('')
    expect(engineLabel({})).toBe('')
  })
})

describe('retryWarning', () => {
  const warn = (trackEngine, current) => retryWarning({ id: 1, engine: trackEngine }, current)

  it('трек yue2.cpp, сейчас PyTorch — предупреждение', () => {
    const s = warn('cpp-q8', 'torch')
    expect(typeof s).toBe('string')
    expect(s).not.toBe('')
  })

  it('трек PyTorch, сейчас yue2.cpp — предупреждение', () => {
    expect(warn('torch', 'cpp-q5')).not.toBe('')
  })

  it('старый трек ("" = PyTorch), сейчас yue2.cpp — предупреждение', () => {
    expect(warn('', 'cpp-q8')).not.toBe('')
    expect(retryWarning({ id: 1 }, 'cpp-q8')).not.toBe('')
  })

  // сид между моделями yue2.cpp не переносится — другая модель = другое исполнение
  it('другая модель yue2.cpp — предупреждение', () => {
    expect(warn('cpp-q8', 'cpp-q5')).not.toBe('')
    expect(warn('cpp-bf16', 'cpp-q8')).not.toBe('')
  })

  it('тот же движок — ничего', () => {
    expect(warn('cpp-q8', 'cpp-q8')).toBe('')
    expect(warn('torch', 'torch')).toBe('')
  })

  it('старый трек ("" или без поля) на PyTorch — ничего', () => {
    expect(warn('', 'torch')).toBe('')
    expect(retryWarning({ id: 1 }, 'torch')).toBe('')
  })

  // конфиг не загружен — текущий движок неизвестен, ложного предупреждения нет
  it('текущий движок неизвестен ("" или undefined) — ничего', () => {
    for (const track of ['cpp-q8', 'torch', '']) {
      expect(warn(track, ''), `трек ${track || '""'}, текущий ""`).toBe('')
      expect(warn(track, undefined), `трек ${track || '""'}, текущий undefined`).toBe('')
    }
  })
})

describe('modelOptions', () => {
  const CFG = {
    engine: 'cpp',
    cpp_model: 'q8',
    cpp_low_vram: false,
    cpp_available: true,
    // порядок ключей нарочно не тот, что в списке
    cpp_models: {
      q5: { file: 'YuE2-3B-Q5_K_M.gguf', size: 2_420_000_000, installed: false },
      q8: { file: 'YuE2-3B-Q8_0.gguf', size: 3_810_232_064, installed: true },
      bf16: { file: 'YuE2-3B-BF16.gguf', size: 7_130_000_000, installed: false },
      q6: { file: 'YuE2-3B-Q6_K.gguf', size: 2_940_000_000, installed: false },
    },
  }

  it('порядок bf16, q8, q6, q5', () => {
    expect(modelOptions(CFG).map((o) => o.value)).toEqual(['bf16', 'q8', 'q6', 'q5'])
  })

  it('признак «скачана» — из installed', () => {
    const byValue = Object.fromEntries(modelOptions(CFG).map((o) => [o.value, o]))
    expect(byValue.q8.installed).toBe(true)
    expect(byValue.bf16.installed).toBe(false)
    expect(byValue.q6.installed).toBe(false)
    expect(byValue.q5.installed).toBe(false)
  })

  it('размер — в ГБ с одним знаком, не в байтах', () => {
    const q8 = modelOptions(CFG).find((o) => o.value === 'q8')
    expect(q8.label).toMatch(/(?<![\d.,])3[.,]8(?![\d])/)
    expect(q8.label).not.toContain('3810232064')
    const bf16 = modelOptions(CFG).find((o) => o.value === 'bf16')
    expect(bf16.label).toMatch(/(?<![\d.,])7[.,]1(?![\d])/)
  })

  it('подпись скачанной и нескачанной модели различается (признак виден в label)', () => {
    const same = (installed) =>
      modelOptions({ cpp_models: { q8: { file: 'YuE2-3B-Q8_0.gguf', size: 3_810_232_064, installed } } })
        .find((o) => o.value === 'q8').label
    expect(same(true)).not.toBe(same(false))
  })

  // модели нет в cpp_models — пропускается
  it('в cpp_models только q8 — один пункт q8', () => {
    const opts = modelOptions({ cpp_models: { q8: { file: 'YuE2-3B-Q8_0.gguf', size: 3_810_232_064, installed: true } } })
    expect(opts.map((o) => o.value)).toEqual(['q8'])
    expect(opts[0].installed).toBe(true)
  })

  it('воркер без cpp_models (старая версия) — пустой список', () => {
    expect(modelOptions({ engine: 'torch' })).toEqual([])
    expect(modelOptions({})).toEqual([])
  })
})

// при основном движке «Сохранить» не шлёт модель yue2.cpp — воркер ответил бы 422
// «модель не скачана» любому пользователю PyTorch без скачанных моделей
describe('engineConfig — что уходит воркеру при сохранении', () => {
  it('основной движок — только engine, без cpp_model', () => {
    expect(engineConfig('torch', 'q8', false)).toEqual({ engine: 'torch' })
    expect(engineConfig('torch', 'q5', true)).toEqual({ engine: 'torch' })
  })
  it('yue2.cpp — с моделью и экономным режимом', () => {
    expect(engineConfig('cpp', 'q6', true)).toEqual({ engine: 'cpp', cpp_model: 'q6', cpp_low_vram: true })
    expect(engineConfig('cpp', 'q8', undefined)).toEqual({ engine: 'cpp', cpp_model: 'q8', cpp_low_vram: false })
  })
})
