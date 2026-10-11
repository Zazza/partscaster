// Тесты превью импорта рецепта трека и списка идущих импортов — по договору этапа 19 (ТК173):
// превью показывает название, число шагов (корень + правки), предупреждения и флаг «модель другая»,
// по которому окно импорта показывает переключатель «по сиду / по плану».
import { describe, it, expect } from 'vitest'
import { recipePreview, activeImports } from './recipe.js'

const rec = (over = {}) => ({
  format: 'yue-track-recipe', version: 1, exported_at: '2026-10-11T10:00:00Z',
  worker_model: 'yue-a', stems_model: 'roformer',
  track: { title: 'Тёплый провод', folder: '', head: 'n1' },
  root: { id: 'n1', title: 'корень', style: 'rock', lyrics: '', seed: 42 },
  ops: [], warnings: [], ...over,
})

const hasRu = (s) => typeof s === 'string' && /[а-яё]/i.test(s)

describe('recipePreview: формат', () => {
  it('принимает объект и тот же рецепт текстом JSON — одинаковое превью', () => {
    const r = rec({ ops: [{ op: 'fx', node: 'n1', params: {}, result: 'x.flac', refs: [] }] })
    const a = recipePreview(r, 'yue-a')
    const b = recipePreview(JSON.stringify(r), 'yue-a')
    expect(a.error).toBeUndefined()
    expect(b).toEqual(a)
  })

  it('кривой JSON → error понятным русским текстом', () => {
    const p = recipePreview('{ это не json', 'yue-a')
    expect(hasRu(p.error)).toBe(true)
    expect(p.steps).toBeUndefined()
  })

  it('чужой format → error', () => {
    expect(hasRu(recipePreview(rec({ format: 'other-thing' }), 'yue-a').error)).toBe(true)
    const noFormat = rec(); delete noFormat.format
    expect(hasRu(recipePreview(noFormat, 'yue-a').error)).toBe(true)
  })

  it('версия ≠ 1 (2, 0, нет версии) → error', () => {
    expect(hasRu(recipePreview(rec({ version: 2 }), 'yue-a').error)).toBe(true)
    expect(hasRu(recipePreview(rec({ version: 0 }), 'yue-a').error)).toBe(true)
    const noVer = rec(); delete noVer.version
    expect(hasRu(recipePreview(noVer, 'yue-a').error)).toBe(true)
  })

  it('пусто: null, пустая строка, не объект → error, не исключение', () => {
    for (const bad of [null, undefined, '', '42', '[]']) {
      const p = recipePreview(bad, 'yue-a')
      expect(hasRu(p.error), String(bad)).toBe(true)
    }
  })
})

describe('recipePreview: название', () => {
  it('title — из track.title', () => {
    expect(recipePreview(rec(), 'yue-a').title).toBe('Тёплый провод')
  })
  it('нет track.title → root.title', () => {
    expect(recipePreview(rec({ track: { title: '', head: 'n1' } }), 'yue-a').title).toBe('корень')
    expect(recipePreview(rec({ track: undefined }), 'yue-a').title).toBe('корень')
  })
  it('нет ни того, ни другого → «трек»', () => {
    const r = rec({ track: { head: 'n1' }, root: { id: 'n1', seed: 1 } })
    expect(recipePreview(r, 'yue-a').title).toBe('трек')
  })
})

describe('recipePreview: шаги и предупреждения', () => {
  it('steps = 1 (корень) + число ops', () => {
    expect(recipePreview(rec({ ops: [] }), 'yue-a').steps).toBe(1)
    const ops = [
      { op: 'continue', node: 'n1', params: {}, result: 'job:n2', refs: [] },
      { op: 'fx', node: 'n2', params: {}, result: 'a.flac', refs: [] },
      { op: 'set_head', node: 'n2', params: {}, result: '', refs: [] },
    ]
    expect(recipePreview(rec({ ops }), 'yue-a').steps).toBe(4)
  })
  it('ops нет в рецепте → steps = 1', () => {
    const r = rec(); delete r.ops
    expect(recipePreview(r, 'yue-a').steps).toBe(1)
  })
  it('warnings — массив из рецепта, порядок сохранён', () => {
    const w = ['версии до журнала не вошли', 'head вне рецепта']
    expect(recipePreview(rec({ warnings: w }), 'yue-a').warnings).toEqual(w)
  })
  it('warnings нет → []', () => {
    const r = rec(); delete r.warnings
    expect(recipePreview(r, 'yue-a').warnings).toEqual([])
  })
})

describe('recipePreview: модель', () => {
  it('model — модель рецепта (worker_model)', () => {
    expect(recipePreview(rec({ worker_model: 'yue-a' }), 'yue-b').model).toBe('yue-a')
  })
  it('обе модели известны и различаются → modelDiffers', () => {
    expect(recipePreview(rec({ worker_model: 'yue-a' }), 'yue-b').modelDiffers).toBe(true)
  })
  it('одинаковые → false', () => {
    expect(recipePreview(rec({ worker_model: 'yue-a' }), 'yue-a').modelDiffers).toBe(false)
  })
  it('модель воркера неизвестна (пусто/null) → false', () => {
    expect(recipePreview(rec({ worker_model: 'yue-a' }), '').modelDiffers).toBe(false)
    expect(recipePreview(rec({ worker_model: 'yue-a' }), null).modelDiffers).toBe(false)
    expect(recipePreview(rec({ worker_model: 'yue-a' }), undefined).modelDiffers).toBe(false)
  })
  it('модель рецепта неизвестна → false', () => {
    expect(recipePreview(rec({ worker_model: '' }), 'yue-b').modelDiffers).toBe(false)
    const r = rec(); delete r.worker_model
    expect(recipePreview(r, 'yue-b').modelDiffers).toBe(false)
  })
})

describe('activeImports', () => {
  it('оставляет только running и error, порядок сохранён', () => {
    const list = [
      { id: 1, status: 'done' }, { id: 2, status: 'error' }, { id: 3, status: 'running' },
      { id: 4, status: 'done' }, { id: 5, status: 'running' }, { id: 6, status: 'queued' },
    ]
    expect(activeImports(list).map((x) => x.id)).toEqual([2, 3, 5])
  })
  it('пустой список, null, undefined → []', () => {
    expect(activeImports([])).toEqual([])
    expect(activeImports(null)).toEqual([])
    expect(activeImports(undefined)).toEqual([])
  })
  it('все готовы → []', () => {
    expect(activeImports([{ id: 1, status: 'done' }])).toEqual([])
  })
})
