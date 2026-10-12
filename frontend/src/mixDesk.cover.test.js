// Хелперы mixDesk без прямых тестов.
//   normPlace / isCenter — место дорожки {pan −1…1, width 0…2} (specs.place); ширина не
//     задана — 1 «как есть» (не 0 «моно»), как Place.UnmarshalJSON в Go; мусор — умолчания;
//   isPlaceRecord — запись реестра «только место»: одна дорожка, без обработки, без рендера;
//   isMasterRecord — запись мастера (master + цепочка движка);
//   stemPlace — место дорожки из реестра; нет записи — центр;
//   premasterFile — «микс без мастера» рядом с миксом студии: overdub-inst-N → overdub-premaster-N
//     (то же правило, что studio.PremasterFile в Go);
//   masterLine — строка о громкости после мастера «−14,0 LUFS, пик −1,0 dBTP»; метрик нет — пусто.
import { describe, it, expect } from 'vitest'
import { normPlace, isCenter, isPlaceRecord, isMasterRecord, stemPlace, withStemPlace, premasterFile, masterLine, CENTER } from './mixDesk.js'

describe('место дорожки в пределах (normPlace, isCenter)', () => {
  it('ничего не задано — центр, ширина 1', () => {
    expect(normPlace(undefined)).toEqual({ pan: 0, width: 1 })
    expect(normPlace(null)).toEqual({ pan: 0, width: 1 })
    expect(normPlace({})).toEqual({ pan: 0, width: 1 })
  })

  it('ширина не задана — 1 (как есть), а не 0 (моно)', () => {
    expect(normPlace({ pan: 0.3 })).toEqual({ pan: 0.3, width: 1 })
  })

  it('явная ширина 0 — моно (не подменяется умолчанием)', () => {
    expect(normPlace({ pan: 0, width: 0 }).width).toBe(0)
  })

  it('за пределами — к краю: pan −1…1, width 0…2', () => {
    expect(normPlace({ pan: 5, width: 9 })).toEqual({ pan: 1, width: 2 })
    expect(normPlace({ pan: -5, width: -1 })).toEqual({ pan: -1, width: 0 })
  })

  it('числа строкой — числами; мусор — умолчания', () => {
    expect(normPlace({ pan: '-0.5', width: '1.4' })).toEqual({ pan: -0.5, width: 1.4 })
    expect(normPlace({ pan: 'лево', width: NaN })).toEqual({ pan: 0, width: 1 })
  })

  it('центр — pan 0 и ширина 1; иначе — не центр', () => {
    expect(isCenter(undefined)).toBe(true)
    expect(isCenter({ pan: 0 })).toBe(true)
    expect(isCenter({ pan: 0.1 })).toBe(false)
    expect(isCenter({ pan: 0, width: 1.2 })).toBe(false)
    expect(isCenter({ pan: 0, width: 0 })).toBe(false)
  })
})

const placeRec = (over = {}) => ({ childId: -5, stems: ['bass'], place: { pan: -0.4, width: 1 }, ...over })

describe('запись «место дорожки» (isPlaceRecord)', () => {
  it('место без обработки на одну дорожку — да', () => {
    expect(isPlaceRecord(placeRec())).toBe(true)
  })

  it('пусто и без места — нет', () => {
    expect(isPlaceRecord(null)).toBe(false)
    expect(isPlaceRecord(undefined)).toBe(false)
    expect(isPlaceRecord(placeRec({ place: null }))).toBe(false)
  })

  it.each([
    ['цепочка движка', { engine: [{ type: 'eq' }] }],
    ['dsp-цепочка', { chain: 'tape' }],
    ['педали', { steps: [] }],
    ['линия громкости', { envelope: [[0, 0]] }],
    ['добавление поверх (синт)', { add: true }],
    ['мастер', { master: true }],
  ])('с обработкой (%s) — нет: место там часть правки', (_, over) => {
    expect(isPlaceRecord(placeRec(over))).toBe(false)
  })

  it('вклейка с рендером (childId > 0) — нет', () => {
    expect(isPlaceRecord(placeRec({ childId: 42 }))).toBe(false)
  })

  it('не одна дорожка — нет', () => {
    expect(isPlaceRecord(placeRec({ stems: [] }))).toBe(false)
    expect(isPlaceRecord(placeRec({ stems: ['bass', 'drums'] }))).toBe(false)
    expect(isPlaceRecord(placeRec({ stems: undefined }))).toBe(false)
  })
})

describe('запись мастера (isMasterRecord)', () => {
  it('мастер с цепочкой — да', () => {
    expect(isMasterRecord({ master: true, engine: [{ type: 'limiter' }] })).toBe(true)
  })

  it('без цепочки, без флага мастера или пусто — нет', () => {
    expect(isMasterRecord({ master: true })).toBe(false)
    expect(isMasterRecord({ engine: [{ type: 'limiter' }] })).toBe(false)
    expect(isMasterRecord(null)).toBe(false)
  })
})

describe('место дорожки из реестра (stemPlace)', () => {
  it('нет записи — центр', () => {
    expect(stemPlace([], 'bass')).toEqual({ pan: 0, width: 1 })
    expect(stemPlace(undefined, 'bass')).toEqual({ pan: 0, width: 1 })
  })

  it('центр отдаётся копией: правка ответа не трогает общий CENTER', () => {
    const p = stemPlace([], 'bass')
    p.pan = 1
    expect(stemPlace([], 'bass')).toEqual({ pan: 0, width: 1 })
    expect(CENTER).toEqual({ pan: 0, width: 1 })
  })

  it('место своей дорожки, не чужой', () => {
    const list = withStemPlace(withStemPlace([], 'bass', { pan: -0.4 }), 'drums', { pan: 0.7, width: 1.5 })
    expect(stemPlace(list, 'bass')).toEqual({ pan: -0.4, width: 1 })
    expect(stemPlace(list, 'drums')).toEqual({ pan: 0.7, width: 1.5 })
    expect(stemPlace(list, 'vocals')).toEqual({ pan: 0, width: 1 })
  })

  it('место внутри правки с обработкой — не «место дорожки»: центр', () => {
    const list = [{ childId: -1, stems: ['bass'], place: { pan: 1 }, engine: [{ type: 'eq' }] }]
    expect(stemPlace(list, 'bass')).toEqual({ pan: 0, width: 1 })
  })

  it('место за пределами в записи — в пределах', () => {
    expect(stemPlace([placeRec({ place: { pan: -3 } })], 'bass')).toEqual({ pan: -1, width: 1 })
  })
})

describe('микс без мастера (premasterFile)', () => {
  it('микс студии → файл рядом без мастера', () => {
    expect(premasterFile('overdub-inst-7.flac')).toBe('overdub-premaster-7.flac')
  })

  it('не микс студии — имя без изменений', () => {
    expect(premasterFile('dsp-fx-mix-ab12.flac')).toBe('dsp-fx-mix-ab12.flac')
    expect(premasterFile('audio.flac')).toBe('audio.flac')
  })

  it('нет файла — пусто', () => {
    expect(premasterFile('')).toBe('')
    expect(premasterFile(null)).toBe('')
    expect(premasterFile(undefined)).toBe('')
  })
})

describe('строка громкости после мастера (masterLine)', () => {
  it('громкость и истинный пик: запятая, настоящий минус, одна цифра после запятой', () => {
    expect(masterLine({ lufs: -14, true_peak_db: -1 })).toBe('−14,0 LUFS, пик −1,0 dBTP')
    expect(masterLine({ lufs: -9.26, true_peak_db: -0.34 })).toBe('−9,3 LUFS, пик −0,3 dBTP')
  })

  it('без пика — только громкость', () => {
    expect(masterLine({ lufs: -14.04 })).toBe('−14,0 LUFS')
  })

  it('положительное значение — без знака', () => {
    expect(masterLine({ lufs: -6, true_peak_db: 0.5 })).toBe('−6,0 LUFS, пик 0,5 dBTP')
  })

  it('метрик нет или громкость не число — пусто', () => {
    expect(masterLine(null)).toBe('')
    expect(masterLine({})).toBe('')
    expect(masterLine({ lufs: '−14' })).toBe('')
    expect(masterLine({ true_peak_db: -1 })).toBe('')
  })
})
