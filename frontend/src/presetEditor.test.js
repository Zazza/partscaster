// Тесты редактора пресета по исходникам: библиотека открывает редактор (новый/правка/копия
// встроенного), редактор собирает готовые кирпичи (ChainEditor, каталог, демо-эталон, плеер),
// модалка следует общему паттерну, словари ru/en синхронны по новым ключам, документация
// описывает редактор. Ловит «редактор сделали, а кнопки в библиотеку забыли».
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const srcDir = dirname(fileURLToPath(import.meta.url))
const repoDir = join(srcDir, '..', '..')
const read = (p) => readFileSync(p, 'utf8')

function templateOf(src) {
  const start = src.indexOf('<template')
  const end = src.lastIndexOf('</template>')
  return start < 0 || end < 0 ? '' : src.slice(start, end)
}
function scriptOf(src) {
  const m = src.match(/<script\b[^>]*>([\s\S]*?)<\/script>/g)
  return m ? m.join('\n') : ''
}
const importsModule = (src, name, file) =>
  new RegExp(`import\\s+${name}\\s+from\\s+['"][^'"]*${file}['"]`).test(scriptOf(src))
// именованный импорт: import { usePresetDemo } from '...'
const importsName = (src, name, file) =>
  new RegExp(`import\\s+\\{[^}]*\\b${name}\\b[^}]*\\}\\s+from\\s+['"][^'"]*${file}['"]`).test(scriptOf(src))
const rendersComponent = (src, name) => {
  const kebab = name.replace(/([a-z])([A-Z])/g, '$1-$2').toLowerCase()
  return new RegExp(`<(${name}|${kebab})[\\s/>]`).test(templateOf(src))
}

const LIBRARY = join(srcDir, 'components', 'PresetLibrary.vue')
const EDITOR = join(srcDir, 'components', 'PresetEditor.vue')
const CATALOG = join(srcDir, 'components', 'PresetCatalog.vue')
const UI_GUIDE = join(repoDir, 'docs', 'ui-guide.md')

// --- библиотека: вход в редактор ---

test('PresetLibrary импортирует и рендерит PresetEditor и кнопку «новый пресет»', () => {
  const src = read(LIBRARY)
  expect(importsModule(src, 'PresetEditor', 'PresetEditor.vue'), 'нет импорта PresetEditor').toBe(true)
  expect(rendersComponent(src, 'PresetEditor'), 'нет <PresetEditor в шаблоне').toBe(true)
  expect(templateOf(src)).toContain(`t('preset.lib.new')`)
})

test('у встроенных — «Сохранить как свой», у своих — «редактор пресета»', () => {
  const tpl = templateOf(read(LIBRARY))
  expect(tpl).toContain(`t('preset.lib.dup.tip')`)
  expect(tpl).toContain(`t('preset.lib.editFull')`)
})

test('у каждого пресета — «послушать на демо» (применить к демо-эталону жанра)', () => {
  const src = read(LIBRARY)
  expect(templateOf(src)).toContain(`t('preset.lib.play.tip')`)
  expect(scriptOf(src)).toContain('applySoundPresetWith')
  expect(importsName(src, 'usePresetDemo', 'usePresetDemo.js')).toBe(true)
})

// --- редактор: кирпичи и модалка ---

test('PresetEditor собирает готовые кирпичи: ChainEditor, каталог, плеер, демо-эталон, PlaceControls', () => {
  const src = read(EDITOR)
  for (const [name, file] of [
    ['ChainEditor', 'ChainEditor.vue'], ['PresetCatalog', 'PresetCatalog.vue'], ['VSelect', 'VSelect.vue'],
    ['PlaceControls', 'PlaceControls.vue'],
  ]) {
    expect(importsModule(src, name, file), `нет импорта ${name}`).toBe(true)
  }
  for (const [name, file] of [
    ['usePresetDemo', 'usePresetDemo.js'], ['usePlayer', 'usePlayer.js'], ['useWindowPlay', 'useWindowPlay.js'],
  ]) {
    expect(importsName(src, name, file), `нет импорта ${name}`).toBe(true)
  }
  expect(rendersComponent(src, 'ChainEditor')).toBe(true)
  expect(rendersComponent(src, 'PresetCatalog')).toBe(true)
})

test('редактор — превью через движок воркера и применение к эталону', () => {
  const src = scriptOf(read(EDITOR))
  expect(src).toContain('applyFx')
  expect(src).toContain('preview: true')
  expect(src).toContain('applySoundPresetWith')
  expect(src).toContain('soundPresetCreate')
  expect(src).toContain('soundPresetUpdate')
})

test('модалка редактора — общий паттерн: backdrop без @click, есть кнопка закрытия', () => {
  const tpl = templateOf(read(EDITOR))
  expect(tpl).toContain('class="modal-backdrop"')
  expect(tpl).not.toMatch(/modal-backdrop[^>]*@click/)
  // закрытие — сразу emit('close') или через обработчик close (подтверждение несохранённых правок, ревью s16)
  expect(tpl).toMatch(/emit\('close'\)|@click="close"/)
})

test('PresetCatalog: табы дорожек и ритм-секция набором, ▶ и «+»', () => {
  const src = read(CATALOG)
  expect(importsName(src, 'EDITOR_TARGETS', 'presetEdit.js')).toBe(true)
  expect(importsName(src, 'DRUM_KITS', 'drumKits.js')).toBe(true)
  expect(scriptOf(src)).toContain('DRUM_TREATMENTS')
  expect(templateOf(src)).toContain(`emit('preview'`)
  expect(templateOf(src)).toContain(`emit('add'`)
})

// --- словари и документация ---

const EDIT_KEYS = [
  'preset.lib.new', 'preset.lib.new.tip', 'preset.lib.editFull', 'preset.lib.dup', 'preset.lib.dup.tip',
  'preset.lib.play', 'preset.lib.play.tip',
  'preset.edit.title', 'preset.edit.new', 'preset.edit.copy', 'preset.edit.save.dup',
  'preset.edit.ref', 'preset.edit.ref.tip', 'preset.edit.ref.demo',
  'preset.edit.demo.cooking', 'preset.edit.demo.stems', 'preset.edit.demo.err', 'preset.edit.demo.again',
  'preset.edit.tracks', 'preset.edit.empty', 'preset.edit.add',
  'preset.edit.spec.chain', 'preset.edit.spec.remove', 'preset.edit.level', 'preset.edit.level.tip',
  'preset.edit.rhythm', 'preset.edit.rhythm.kit', 'preset.edit.rhythm.treat', 'preset.edit.rhythm.add',
  'preset.edit.master', 'preset.edit.parts', 'preset.edit.apply', 'preset.edit.apply.tip',
  'preset.edit.applied', 'preset.edit.window', 'preset.edit.needRoformer', 'preset.edit.noStems',
]

test.each([
  ['ru', RU],
  ['en', EN],
])('словарь %s: ключи редактора непусты', (_lang, dict) => {
  for (const key of EDIT_KEYS) {
    expect(typeof dict[key], `нет ключа ${key}`).toBe('string')
    expect(dict[key].trim().length, `пустой ${key}`).toBeGreaterThan(0)
  }
})

test('словари ru и en синхронны: ключи preset.edit.* и preset.lib.* совпадают', () => {
  const sel = (dict) => Object.keys(dict).filter((k) => k.startsWith('preset.edit.') || k.startsWith('preset.lib.')).sort()
  expect(sel(EN)).toEqual(sel(RU))
})

test('docs/ui-guide.md: раздел «Пресеты звука» описывает редактор и демо-эталоны', () => {
  const lines = read(UI_GUIDE).split('\n')
  const out = []
  for (const line of lines) {
    const m = line.match(/^##\s+(.*)$/)
    if (m) out.push({ title: m[1].trim(), body: '' })
    else if (out.length) out[out.length - 1].body += line + '\n'
  }
  const sec = out.find((s) => s.title.startsWith('Пресеты звука'))
  expect(sec, 'нет раздела «## Пресеты звука»').toBeTruthy()
  expect(sec.body).toMatch(/редактор/i)
  expect(sec.body).toMatch(/демо/i)
})
