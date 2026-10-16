// Каждая фаза генерации, которую воркер отдаёт в stage (counters["phase"] = "…", phase="…"), подписана
// в обоих словарях: иначе в очереди и на карточке видно сырой ключ «queue.progress.arc-plan».
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const workerDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'worker')

function phases() {
  const out = new Set()
  for (const f of ['yue_worker.py', 'engine_cpp.py']) {
    const src = readFileSync(join(workerDir, f), 'utf8')
    for (const m of src.matchAll(/counters\["phase"\]\s*=\s*"([\w-]+)"|phase="([\w-]+)"|"phase":\s*"([\w-]+)"/g)) {
      out.add(m[1] || m[2] || m[3])
    }
    // фазы из выражений: new_phase = "plan" if … else "semantic"; stage = "decode"
    for (const m of src.matchAll(/(?:new_phase|stage)\s*=\s*"([\w-]+)"(?:\s+if[^\n]*else\s+"([\w-]+)")?/g)) {
      out.add(m[1])
      if (m[2]) out.add(m[2])
    }
  }
  return [...out]
}

test('фазы воркера найдены (разбор не сломался)', () => {
  expect(phases()).toEqual(expect.arrayContaining(['load', 'plan', 'semantic', 'decode', 'finalize', 'arc-plan']))
})

test('у каждой фазы есть подпись queue.progress.* в ru и en', () => {
  for (const ph of phases()) {
    expect(RU['queue.progress.' + ph], `ru: queue.progress.${ph}`).toBeTruthy()
    expect(EN['queue.progress.' + ph], `en: queue.progress.${ph}`).toBeTruthy()
  }
})
