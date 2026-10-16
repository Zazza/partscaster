// Подпись записи реестра в списке правок (insertTitle) — записи «на весь трек» и линии громкости.
// Карточка internal-studio-draft2, по итогам ревью: подпись записи (label) важнее названия цепочки;
// линия громкости — «линия громкости · <дорожка>» через ключ i18n insert.envelope.
// Язык — русский по умолчанию (в node нет localStorage).
import { describe, it, expect } from 'vitest'
import { insertTitle } from './insertLabels.js'

const names = {
  chainName: (c) => ({ master: 'Мастеринг', soften: 'Смягчить звон' })[c] || c,
  stemName: (s) => ({ drums: 'барабаны', mix: 'весь трек', vocals: 'голос' })[s] || s,
  instName: (id) => 'приём ' + id,
}

describe('insertTitle — запись с chain', () => {
  it('с label — подпись начинается с label, а не с названия цепочки', () => {
    const got = insertTitle({ childId: -5, chain: 'master', label: 'Мастеринг (средне)', stems: ['mix'], mix: true }, names)
    expect(got.startsWith('Мастеринг (средне)')).toBe(true)
  })

  it('без label — название цепочки (chainName)', () => {
    const got = insertTitle({ childId: -5, chain: 'master', stems: ['mix'], mix: true }, names)
    expect(got.startsWith('Мастеринг')).toBe(true)
    expect(got.startsWith('Мастеринг (')).toBe(false)
  })
})

describe('insertTitle — линия громкости', () => {
  const env = [{ t: 1, db: 0 }, { t: 3, db: -12 }]

  it('на дорожку — «линия громкости · барабаны»', () => {
    const got = insertTitle({ childId: -6, instId: 'env', stems: ['drums'], envelope: env }, names)
    expect(got).toBe('линия громкости · барабаны')
  })

  it('на весь трек — «линия громкости · весь трек»', () => {
    const got = insertTitle({ childId: -7, stems: ['mix'], mix: true, envelope: env }, names)
    expect(got).toBe('линия громкости · весь трек')
  })

  it('сырой ключ studio.trick.inst.env в подпись не попадает', () => {
    for (const it of [{ childId: -6, instId: 'env', stems: ['drums'], envelope: env },
      { childId: -7, stems: ['mix'], mix: true, envelope: env }]) {
      expect(insertTitle(it, names)).not.toContain('studio.trick.inst.env')
    }
  })
})
