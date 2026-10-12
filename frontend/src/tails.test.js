// Паритет данных MCP локально: internal/mcp/fx_presets.json равен генерации из fxPresets.js
//     (JSON.stringify(fxPresets, null, 1)); internal/mcp/fx_blocks.json и frontend/src/fxBlocks.json —
//     побайтно worker/fx_blocks.json;
// rhythmSection(['kick'], fxPresets, false, 'en'): подпись на английском (имя пресета en).
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fxPresets } from './fxPresets.js'
import { rhythmSection } from './trackDesk.js'

const repo = (rel) => new URL(`../../${rel}`, import.meta.url)
const bytes = (rel) => readFileSync(repo(rel))

describe('паритет данных MCP', () => {
  it('internal/mcp/fx_presets.json == JSON.stringify(fxPresets, null, 1)', () => {
    expect(bytes('internal/mcp/fx_presets.json').toString('utf8')).toBe(JSON.stringify(fxPresets, null, 1))
  })

  it.each(['internal/mcp/fx_blocks.json', 'frontend/src/fxBlocks.json'])('%s побайтно равен worker/fx_blocks.json', (rel) => {
    expect(bytes(rel).equals(bytes('worker/fx_blocks.json'))).toBe(true)
  })
})

describe('rhythmSection с locale en', () => {
  it("['kick'], locale 'en' — подпись на английском: имя пресета en, без кириллицы", () => {
    const out = rhythmSection(['kick'], fxPresets, false, 'en')
    expect(out.length).toBe(1)
    const kit = fxPresets.find((p) => p.id === 'drums-kick-kit')
    expect(kit).toBeDefined()
    expect(out[0].label).toContain(kit.name.en)
    expect(out[0].label).not.toMatch(/[а-яё]/i)
  })
})
