// Звуковой движок из интерфейса без Vue: докачка наборов сэмплов (превью) и запись цепочки в черновик.
// Вызывают пульт дорожек студии (TrackDesk), синт, перкуссия и InstrumentsPage.
import { missingKits } from './fxChain.js'
import { installKit } from './kitProgress.js'

const copy = (v) => JSON.parse(JSON.stringify(v)) // цепочка — данные JSON: копия без общих объектов

/** Блоки, чьи наборы нужны для расчёта: с обработкой — вся цепочка; без неё (bypass) воркер играет только блоки,
 *  которые сами дают ноты (synth, perc, gtrnotes), — наборы эффектов не нужны и не качаются. */
const NOTE_BLOCKS = ['synth', 'perc', 'gtrnotes']
export function kitChain(chain, bypass) {
  return bypass ? chain.filter((b) => NOTE_BLOCKS.includes(b.type)) : chain
}

/** Наборы цепочки (kit/kit_open/kit_mid/kit_low у sampler, kit у bass и perc, kit/mute_kit у gtrnotes), которых нет среди kits воркера, — ставятся
 *  по одному, в порядке цепочки; onKit(имя) — перед установкой (подпись «качаю набор…»). */
export async function ensureKits(api, chain, kits, onKit) {
  const need = missingKits(chain, kits)
  for (const k of need) {
    if (onKit) onKit(k)
    await installKit(api, k)   // прогресс — KitProgress на странице
  }
  return need
}

/** Запись цепочки движка на дорожку для useInserts.addStemEngine из снимка {src, from, to, chain, label}: только
 *  данные — без запроса к воркеру; наборы ставит и цепочку проверяет сборка (сбой — красная строка правки). */
export function engineRecord(snap) {
  return { stem: snap.src, chain: copy(snap.chain || []), from: snap.from, to: snap.to, label: snap.label }
}
