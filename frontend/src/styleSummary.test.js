// Короткий стиль трека в строке списка (пачка 9, ТК20): до n фраз в порядке
// полей формы, без языка и темпа; пусто — пустая строка.
import { describe, it, expect } from 'vitest'
import { styleSummary } from './styleTags.js'

const STYLE = 'English, post-punk, baritone male vocals, driving bass, melancholic, 110 BPM'

describe('styleSummary: отбор и порядок', () => {
  it('три фразы через « · » без языка и BPM, в порядке полей формы', () => {
    // порядок формы: жанр → гитары/бас → голос → настроение
    expect(styleSummary(STYLE)).toBe('post-punk · driving bass · baritone male vocals')
  })

  it('язык и темп не попадают в итог', () => {
    const s = styleSummary(STYLE, 10)
    expect(s).not.toMatch(/English/)
    expect(s).not.toMatch(/BPM/)
    expect(s.split(' · ')).toHaveLength(4)
  })

  it('жанр раньше голоса, даже если в строке стиля голос стоит первым', () => {
    expect(styleSummary('baritone male vocals, post-punk', 3)).toBe('post-punk · baritone male vocals')
  })

  it('n=1 — одна фраза, первая по порядку формы', () => {
    expect(styleSummary(STYLE, 1)).toBe('post-punk')
  })
})

describe('styleSummary: пустые случаи', () => {
  it("'' и null — пустая строка", () => {
    expect(styleSummary('')).toBe('')
    expect(styleSummary(null)).toBe('')
  })

  it('только язык и темп — пустая строка', () => {
    expect(styleSummary('English, 120 BPM')).toBe('')
  })
})
