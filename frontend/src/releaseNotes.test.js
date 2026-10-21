import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { RELEASE_NOTES } from './releaseNotes.js'

// Страж шага релиза: версия вышла в CHANGELOG — в окне
// «Что нового» должна быть её запись. Иначе приложение не узнает о новой версии.
const num = (v) => v.split('.').map(Number)
const newer = (a, b) => {
  const [x, y] = [num(a), num(b)]
  for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return x[i] > y[i]
  return false
}

describe('releaseNotes: запись на каждую вышедшую версию', () => {
  it('первая запись не старше последней версии в CHANGELOG', () => {
    const changelog = readFileSync(new URL('../../CHANGELOG.md', import.meta.url), 'utf8')
    const released = [...changelog.matchAll(/^## \[(\d+\.\d+\.\d+)\]/gm)].map((m) => m[1])
    expect(released.length).toBeGreaterThan(0)
    const latest = released.reduce((a, b) => (newer(b, a) ? b : a))
    expect(newer(latest, RELEASE_NOTES[0].version),
      `в CHANGELOG вышла ${latest}, а первая запись releaseNotes.js — ${RELEASE_NOTES[0].version}`).toBe(false)
  })
})

// Пока раздел 1.0.0 в CHANGELOG помечен «не выпущена», 1.0.0 — текущая готовящаяся
// версия: первая запись и пустой Unreleased обязательны. После релиза (заголовок с датой)
// эти две проверки пропускаются — следующая версия законно встаёт первой.
const header100 = readFileSync(new URL('../../CHANGELOG.md', import.meta.url), 'utf8')
  .split('\n').find((l) => l.startsWith('## [1.0.0]')) ?? ''
const released100 = !header100.includes('не выпущена')

// ТК1 карточки internal-release-notes-1-0: «Что нового» для 1.0.0.
describe('releaseNotes: запись 1.0.0', () => {
  it.skipIf(released100)('первая запись — 1.0.0', () => {
    expect(RELEASE_NOTES[0].version).toBe('1.0.0')
  })

  it('записи 0.8.0 нет (версия не выходила)', () => {
    expect(RELEASE_NOTES.map((n) => n.version)).not.toContain('0.8.0')
  })

  it('ru и en одной длины, от 5 до 15 пунктов', () => {
    const { ru, en } = RELEASE_NOTES[0]
    expect(Array.isArray(ru)).toBe(true)
    expect(Array.isArray(en)).toBe(true)
    expect(ru.length).toBe(en.length)
    expect(ru.length).toBeGreaterThanOrEqual(5)
    expect(ru.length).toBeLessThanOrEqual(15)
  })

  it('пункты непустые', () => {
    const { ru, en } = RELEASE_NOTES[0]
    for (const item of [...ru, ...en]) {
      expect(typeof item).toBe('string')
      expect(item.trim().length).toBeGreaterThan(0)
    }
  })
})

// ТК2: CHANGELOG — раздел 1.0.0 сведён в одну группу заголовков, Unreleased пуст.
describe('CHANGELOG: раздел 1.0.0 сведён', () => {
  const changelog = readFileSync(new URL('../../CHANGELOG.md', import.meta.url), 'utf8')
  const lines = changelog.split('\n')

  // Строки раздела от заголовка '## [ver]' до следующего '## [' (без самого заголовка).
  const section = (ver) => {
    const start = lines.findIndex((l) => l.startsWith(`## [${ver}]`))
    if (start === -1) return null
    let end = lines.findIndex((l, i) => i > start && l.startsWith('## ['))
    if (end === -1) end = lines.length
    return lines.slice(start + 1, end)
  }

  // Пункты '- ' внутри подраздела '### name' раздела.
  const items = (sec, name) => {
    const start = sec.findIndex((l) => l.trim() === `### ${name}`)
    if (start === -1) return []
    let end = sec.findIndex((l, i) => i > start && l.startsWith('### '))
    if (end === -1) end = sec.length
    return sec.slice(start + 1, end).filter((l) => l.startsWith('- '))
  }

  it('раздел 1.0.0 есть', () => {
    expect(section('1.0.0')).not.toBeNull()
  })

  it.each(['Added', 'Changed', 'Fixed', 'Removed'])('### %s встречается не больше одного раза', (name) => {
    const sec = section('1.0.0') ?? []
    const count = sec.filter((l) => l.trim() === `### ${name}`).length
    expect(count).toBeLessThanOrEqual(1)
  })

  it.skipIf(released100)('раздела [Unreleased] с содержимым нет', () => {
    const sec = section('Unreleased')
    if (sec === null) return
    expect(sec.filter((l) => l.trim() !== '').join('\n')).toBe('')
  })

  it.each(['Added', 'Fixed'])('в ### %s раздела 1.0.0 есть хотя бы один пункт', (name) => {
    expect(items(section('1.0.0') ?? [], name).length).toBeGreaterThan(0)
  })
})
