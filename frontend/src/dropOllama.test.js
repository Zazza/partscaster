// internal-drop-ollama: Ollama (копайтер, перевод стиля, адаптация текста) и «из трека» удалены
// со слоя фронта; над полем стихов — кнопки тегов секций; кириллица в стиле не переводится,
// а помечается предупреждением. Проверка по исходникам, как в importTrack.test.js.
import { readFileSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'
import { hasCyrillic } from './requests.js'
import * as requests from './requests.js'
import * as voiceLab from './voiceLab.js'

const srcDir = dirname(fileURLToPath(import.meta.url))
const read = (p) => readFileSync(p, 'utf8')

const APP = join(srcDir, 'App.vue')
const API = join(srcDir, 'api.js')
const VOICES = join(srcDir, 'components', 'VoicesPage.vue')
const REPLAY = join(srcDir, 'components', 'ReplayModal.vue')
const SETTINGS = join(srcDir, 'components', 'SettingsPage.vue')
const STUDIO = join(srcDir, 'components', 'StudioPage.vue')
const COPILOT_MODAL = join(srcDir, 'components', 'CopilotModal.vue')
const COPILOT_PROMPT = join(srcDir, 'copilotPrompt.js')
const BINDINGS_JS = join(srcDir, 'wailsjs', 'go', 'main', 'App.js')
const BINDINGS_DTS = join(srcDir, 'wailsjs', 'go', 'main', 'App.d.ts')

function templateOf(src) {
  const start = src.indexOf('<template')
  const end = src.lastIndexOf('</template>')
  return start < 0 || end < 0 ? '' : src.slice(start, end)
}

function scriptOf(src) {
  const m = src.match(/<script\b[^>]*>([\s\S]*?)<\/script>/g)
  return m ? m.join('\n') : ''
}

const tKeys = (s) => [...s.matchAll(/\bt\(\s*['"]([\w.-]+)['"]/g)].map((m) => m[1])

// Элемент, открывающий тег которого начинается в позиции idx: до парного закрывающего
// тега того же имени (с учётом вложенности одноимённых).
function elementAt(tpl, idx) {
  const name = /^<([\w-]+)/.exec(tpl.slice(idx))[1]
  const re = new RegExp(`<${name}\\b(?:[^>"']|"[^"]*"|'[^']*')*?(/?)>|</${name}\\s*>`, 'g')
  re.lastIndex = idx
  let depth = 0
  let x
  while ((x = re.exec(tpl))) {
    if (x[0].startsWith('</')) depth--
    else if (!x[1]) depth++
    else if (depth === 0) return x[0] // самозакрывающийся
    if (depth === 0) return tpl.slice(idx, x.index + x[0].length)
  }
  return tpl.slice(idx)
}

// Кнопки блока: открывающий тег с атрибутами + содержимое до </button>, с позицией.
function buttonsOf(block) {
  const out = []
  const re = /<button\b((?:[^>"']|"[^"]*"|'[^']*')*)>([\s\S]*?)<\/button>/g
  let m
  while ((m = re.exec(block))) out.push({ attrs: m[1], body: m[2], index: m.index, raw: m[0] })
  return out
}

const clickOf = (attrs) => (/@click="([^"]*)"/.exec(attrs) || [])[1] || ''

// Тело функции скрипта по имени: от объявления до следующего объявления верхнего уровня.
function fnBody(script, name) {
  const decl = new RegExp(
    `(?:^|\\n)(?:async\\s+)?function\\s+${name}\\s*\\(|(?:^|\\n)const\\s+${name}\\s*=`)
  const m = decl.exec(script)
  if (!m) return ''
  const rest = script.slice(m.index + m[0].length)
  const next = /\n(?:async\s+function|function|const|let|var|watch|onMounted)\b/.exec(rest)
  return next ? rest.slice(0, next.index) : rest
}

// Ведёт ли выражение к вызову insertSectionTag: прямо или через функции скрипта (до 3 ступеней).
function leadsTo(callee, expr, script, depth = 0) {
  if (new RegExp(`\\b${callee}\\s*\\(`).test(expr)) return true
  if (depth > 2) return false
  for (const [, name] of expr.matchAll(/\b([A-Za-z_$][\w$]*)\s*(?:\(|$)/g)) {
    const body = fnBody(script, name)
    if (body && leadsTo(callee, body, script, depth + 1)) return true
  }
  return false
}

// Участок формы от подписи поля стихов до самого поля (textarea с v-model="lyrics").
function lyricsRegion() {
  const tpl = templateOf(read(APP))
  const label = tpl.indexOf("t('form.lyrics')")
  const ta = /<textarea\b[^>]*v-model="lyrics"[^>]*>/.exec(tpl)
  return { tpl, label, taIndex: ta ? ta.index : -1, taEnd: ta ? ta.index + ta[0].length : -1 }
}

function tagButtons() {
  const src = read(APP)
  const script = scriptOf(src)
  const { tpl, label, taIndex } = lyricsRegion()
  const region = label >= 0 && taIndex > label ? tpl.slice(label, taIndex) : ''
  const buttons = buttonsOf(region).filter((b) => leadsTo('insertSectionTag', clickOf(b.attrs), script))
  return { src, script, tpl, region, regionStart: label, buttons }
}

describe('условие 4: кнопки тегов над полем стихов', () => {
  test('App.vue берёт insertSectionTag и SECTION_TAGS из lyricsTags.js', () => {
    const script = scriptOf(read(APP))
    expect(script).toMatch(/import\s*\{[^}]*\binsertSectionTag\b[^}]*\}\s*from\s*['"]\.\/lyricsTags(\.js)?['"]/)
    expect(script).toMatch(/import\s*\{[^}]*\bSECTION_TAGS\b[^}]*\}\s*from\s*['"]\.\/lyricsTags(\.js)?['"]/)
  })

  test('между подписью поля стихов и самим полем есть кнопка, которая вызывает insertSectionTag', () => {
    const { region, buttons } = tagButtons()
    expect(region, 'участок от t(\'form.lyrics\') до <textarea v-model="lyrics">').not.toBe('')
    expect(buttons.length, 'кнопка тега над полем стихов').toBeGreaterThan(0)
  })

  test('кнопки строятся по SECTION_TAGS (все семь тегов), подпись — lyrics.tag.<id> через t()', () => {
    const { buttons, region } = tagButtons()
    expect(buttons.length).toBeGreaterThan(0)
    const b = buttons[0]
    // v-for по SECTION_TAGS — на кнопке или на её обёртке в том же участке
    expect(/v-for="[^"]*\bSECTION_TAGS\b[^"]*"/.test(b.attrs) ||
      /v-for="[^"]*\bSECTION_TAGS\b[^"]*"/.test(region), 'v-for по SECTION_TAGS').toBe(true)
    expect(b.raw, 'подпись — t(\'lyrics.tag.\' + id)').toMatch(/\bt\(\s*[`'"]lyrics\.tag\./)
  })

  test('всплывающая подсказка кнопки — что вставится (сам тег)', () => {
    const { buttons } = tagButtons()
    expect(buttons.length).toBeGreaterThan(0)
    const title = (/:title="([^"]*)"/.exec(buttons[0].attrs) || [])[1] || ''
    expect(title, 'у кнопки привязан :title').not.toBe('')
    expect(title, ':title ссылается на тег (что вставится)').toMatch(/\.tag\b/)
  })

  test('кнопки скрыты в режиме «без слов» (v-if/v-show с !noLyrics на кнопке или обёртке)', () => {
    const { tpl, buttons, regionStart } = tagButtons()
    expect(buttons.length).toBeGreaterThan(0)
    const btnAt = regionStart + buttons[0].index
    const cond = /v-(?:if|show)="[^"]*!\s*noLyrics\b[^"]*"/
    if (cond.test(buttons[0].attrs)) return
    // ищем обёртку с условием, внутри которой лежит кнопка
    const re = /<([\w-]+)\b(?:[^>"']|"[^"]*"|'[^']*')*>/g
    let m
    let hidden = false
    while ((m = re.exec(tpl)) && m.index < btnAt) {
      if (!cond.test(m[0])) continue
      const el = elementAt(tpl, m.index)
      if (m.index + el.length > btnAt) { hidden = true; break }
    }
    expect(hidden, 'кнопка тегов или её обёртка не скрыта при noLyrics').toBe(true)
  })

  test('после вставки поле в фокусе и курсор ставится (focus + setSelectionRange)', () => {
    const script = scriptOf(read(APP))
    expect(script).toMatch(/\.focus\(\s*\)/)
    expect(script).toMatch(/\.setSelectionRange\(/)
  })
})

describe('условие 5: подсказка — теги необязательны', () => {
  test('у поля стихов текст i18n: без тегов YuE2 расставит части сама, строфы — через пустую строку', () => {
    const { tpl, label, taEnd } = lyricsRegion()
    expect(label).toBeGreaterThanOrEqual(0)
    expect(taEnd).toBeGreaterThan(label)
    // подпись/подсказка — от подписи поля до 1500 символов после textarea
    const keys = tKeys(tpl.slice(label, taEnd + 1500))
    const hint = keys.filter((k) => /необязательн/i.test(RU[k] || '') && /пуст\S*\s+строк/i.test(RU[k] || ''))
    expect(hint.length, `ключ-подсказка среди ${keys.join(', ')}`).toBeGreaterThan(0)
    for (const k of hint) {
      expect(EN[k], `en: ${k}`).toBeTruthy()
      expect(EN[k]).toMatch(/optional/i)
    }
  })
})

describe('условие 2: кириллица в стиле — предупреждение вместо перевода', () => {
  test('hasCyrillic: русские буквы (включая ё/Ё) — да', () => {
    expect(hasCyrillic('мрачный панк')).toBe(true)
    expect(hasCyrillic('punk, Ёлка')).toBe(true)
    expect(hasCyrillic('ё')).toBe(true)
    expect(hasCyrillic('dark punk, хриплый')).toBe(true)
  })

  test('hasCyrillic: латиница, цифры, пусто — нет', () => {
    expect(hasCyrillic('dark punk, 120 BPM')).toBe(false)
    expect(hasCyrillic('[Verse] female vocals, C# minor')).toBe(false)
    expect(hasCyrillic('')).toBe(false)
  })

  test('hasCyrillic: нет строки (стиль ещё не собран) — нет, без исключения', () => {
    expect(hasCyrillic(undefined)).toBe(false)
    expect(hasCyrillic(null)).toBe(false)
  })

  test('App.vue: под строкой стиля предупреждение по hasCyrillic(стиль), текст из i18n ru/en', () => {
    const src = read(APP)
    expect(scriptOf(src)).toMatch(/import\s*\{[^}]*\bhasCyrillic\b[^}]*\}\s*from\s*['"]\.\/requests(\.js)?['"]/)
    const tpl = templateOf(src)
    const m = /<([\w-]+)\b[^>]*v-(?:if|show)="[^"]*\bhasCyrillic\(\s*[^"]*[Ss]tyle[^"]*"[^>]*>/.exec(tpl)
    expect(m, 'элемент с v-if="hasCyrillic(…стиль…)"').toBeTruthy()
    const el = elementAt(tpl, m.index)
    const keys = tKeys(el)
    expect(keys.length, 'текст предупреждения через t(…)').toBeGreaterThan(0)
    for (const k of keys) {
      expect(RU[k], `ru: ${k}`).toBeTruthy()
      expect(EN[k], `en: ${k}`).toBeTruthy()
    }
    expect(keys.some((k) => /англ/i.test(RU[k])), 'ru: «напишите по-английски»').toBe(true)
    expect(keys.some((k) => /english/i.test(EN[k])), 'en: «write in English»').toBe(true)
  })

  test('предупреждение не блокирует отправку: hasCyrillic не участвует в :disabled', () => {
    const tpl = templateOf(read(APP))
    expect(tpl).not.toMatch(/:disabled="[^"]*hasCyrillic/)
  })

  for (const [name, file] of [['примерочная голосов', VOICES], ['«переиграть в другом стиле…»', REPLAY]]) {
    test(`${name}: свои слова не переводятся, при кириллице — предупреждение i18n`, () => {
      const src = read(file)
      expect(src).not.toMatch(/api\.translate\b/)
      expect(src).not.toMatch(/\bneedsTranslate\b/)
      expect(src).toMatch(/\bhasCyrillic\b/)
      const tpl = templateOf(src)
      const m = /<([\w-]+)\b[^>]*v-(?:if|show)="[^"]*\bhasCyrillic\([^"]*"[^>]*>/.exec(tpl)
      expect(m, 'элемент с v-if="hasCyrillic(…)"').toBeTruthy()
      const keys = tKeys(elementAt(tpl, m.index))
      expect(keys.length).toBeGreaterThan(0)
      for (const k of keys) {
        expect(RU[k], `ru: ${k}`).toBeTruthy()
        expect(EN[k], `en: ${k}`).toBeTruthy()
      }
    })
  }
})

describe('условие 1: Ollama удалена со слоя фронта', () => {
  test('нет CopilotModal.vue и copilotPrompt.js', () => {
    expect(existsSync(COPILOT_MODAL)).toBe(false)
    expect(existsSync(COPILOT_PROMPT)).toBe(false)
  })

  test('App.vue без копайтера, перевода стиля и «из трека»', () => {
    const src = read(APP)
    for (const name of ['CopilotModal', 'copilotPrompt', 'copOpen', 'onCopInsert', 'autoTranslate',
      'needsTranslate', 'translateOr', 'translateBusy', 'adaptLyrics', 'recognizeLyrics', 'jobLyrics']) {
      expect(src, name).not.toMatch(new RegExp(`\\b${name}\\b`))
    }
    expect(src).not.toMatch(/api\.translate\b/)
  })

  test('requests.js и voiceLab.js не экспортируют needsTranslate/translateOr', () => {
    expect(requests.needsTranslate).toBeUndefined()
    expect(requests.translateOr).toBeUndefined()
    expect(voiceLab.needsTranslate).toBeUndefined()
  })

  test('api.js без copilot/translate/adaptLyrics/ollamaModels/recognizeLyrics/jobLyrics', () => {
    const src = read(API)
    for (const name of ['copilot', 'translate', 'adaptLyrics', 'ollamaModels', 'recognizeLyrics', 'jobLyrics']) {
      expect(src, `api.${name}`).not.toMatch(new RegExp(`\\b${name}\\s*:`))
    }
    expect(src).not.toMatch(/Yue(Copilot|Translate|AdaptLyrics|OllamaModels|RecognizeLyrics\w*|JobLyrics)\b/)
  })

  test('привязки Wails App.js / App.d.ts без удалённых биндингов', () => {
    for (const f of [BINDINGS_JS, BINDINGS_DTS]) {
      if (existsSync(f)) {
        expect(read(f), f).not.toMatch(/Yue(Copilot|Translate|AdaptLyrics|OllamaModels|RecognizeLyrics\w*|JobLyrics)\b/)
      }
    }
  })

  test('страница настроек без раздела Ollama', () => {
    expect(read(SETTINGS)).not.toMatch(/ollama/i)
  })

  test('студия без whisperOk / api.jobLyrics', () => {
    const src = read(STUDIO)
    expect(src).not.toMatch(/\bwhisperOk\b/)
    expect(src).not.toMatch(/\bjobLyrics\b/)
  })

  test('в словарях ru/en нет ключей копайтера, перевода, адаптации и «из трека»', () => {
    const banned = ['copilot.', 'form.copilot', 'lyrics.adapt', 'lyrics.rec', 'lyrics.job',
      'form.autotr', 'form.translat', 'settings.ollama']
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      const left = Object.keys(dict).filter((k) => banned.some((p) => k.startsWith(p)))
      expect(left, lang).toEqual([])
    }
  })

  test('в текстах ru/en Ollama не упоминается', () => {
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      const left = Object.keys(dict).filter((k) => /ollama/i.test(String(dict[k])))
      expect(left, lang).toEqual([])
    }
  })
})
