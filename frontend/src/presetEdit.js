// Редактор пресета звука (страница «Пресеты звука») — чистая логика без DOM: модель пресета для
// правок и тело для сохранения, каталог готовых цепочек по дорожкам, дефолты записей, источник
// предпрослушивания, «ритм-секция набором» в записи пресета. Компоненты — PresetEditor/PresetCatalog.
import { rhythmSection } from './trackDesk.js'
import { groupPresets } from './presetGroups.js'
import { fxPresets } from './fxPresets.js'
import BLOCKS from './fxBlocks.json'
import { fillAmp, fromWorkerChain, toWorkerChain } from './fxChain.js'

// табы редактора — по одной дорожке (порядок = порядок вкладок)
export const EDITOR_TARGETS = ['vocals', 'drums', 'bass', 'guitar', 'piano', 'other']

// части барабанов: отдельные дорожки RoFormer, в табе «Барабаны» обрабатываются набором сэмплов
export const DRUM_PARTS = ['kick', 'snare', 'toms', 'hh', 'ride', 'crash']

// дорожки превью в порядке приоритета: основная дорожка звучит представительнее части барабанов
const PREVIEW_ORDER = ['vocals', 'drums', 'bass', 'guitar', 'piano', 'other']

// у записи пресета может быть только один вид обработки — порядок как в PresetSpec
const SPEC_KINDS = ['engine', 'chain', 'steps']

/** Партийный пресет каталога (первый блок — synth/perc без нот) — в пресете звука молчит: не для редактора. */
export function isPartItem(p) {
  const first = (p.chain || [])[0]
  return !!first && (first.type === 'synth' || first.type === 'perc')
}

/** Пресет подходит табу: стемы включают дорожку таба (у «Барабанов» — и любую часть барабанов);
 *  партийные и мастер — мимо, пресеты без стемов — тоже (это мастера: у master-glue стемов нет). */
export function itemForTarget(p, target) {
  if (isPartItem(p) || !(p.stems || []).length) return false
  const want = target === 'drums' ? ['drums', ...DRUM_PARTS] : [target]
  return (p.stems || []).some((s) => want.includes(s) && s !== 'master')
}

/** Пресеты для секции «Мастер»: стем 'master' или без стемов вовсе (master-glue). */
export function masterPresets(presets = fxPresets) {
  return (presets || []).filter((p) => (p.stems || []).includes('master') || !(p.stems || []).length)
}

/** Каталог для таба: [{group, label, items}] по группам (groupPresets), партийные и мастер отсеяны. */
export function editorCatalog(target, presets = fxPresets, locale = 'ru') {
  return groupPresets((presets || []).filter((p) => itemForTarget(p, target)), locale)
}

/** Стемы записи при добавлении пресета из каталога: пересечение стемов пресета с дорожками таба;
 *  пусто (универсальный пресет) — дорожка таба. Гитарные пресеты дают ['guitar'], как во встроенных. */
export function specStemsFor(item, target) {
  const want = target === 'drums' ? ['drums', ...DRUM_PARTS] : [target]
  const hit = (item.stems || []).filter((s) => want.includes(s))
  return hit.length ? hit : [target]
}

/** Запись пресета из пресета каталога: стемы по табу, цепочка движка (усилителю без захвата —
 *  первый загруженный по подсказке amp_hint), нейтральное место, без цели громкости. */
export function specFromItem(item, target, amps = []) {
  // fillAmp работает в форме редактора: конвертация туда-обратно, как в пульте дорожек
  const edit = fromWorkerChain(item.chain || [], BLOCKS)
  return {
    uid: nextUid(),
    stems: specStemsFor(item, target),
    engine: toWorkerChain(fillAmp(edit, amps, item.amp_hint)),
    db: 0,
    place: { ...NEUTRAL_PLACE },
    levelDb: null,
  }
}

/** Вид обработки записи: 'engine' | 'chain' | 'steps' | '' (только место/громкость). */
export function specKind(spec) {
  return SPEC_KINDS.find((k) => spec[k] && (!Array.isArray(spec[k]) || spec[k].length)) || ''
}

/** На частях барабанов пересборка применяет только цепочку движка: эффект ffmpeg или педали там
 *  молча пропали бы — применение пресета отказалось бы с ошибкой (internal/studio/presets.go). */
export function specKindOk(stems, kind) {
  const drumPart = (stems || []).some((s) => DRUM_PARTS.includes(s))
  return !drumPart || kind === 'engine' || kind === ''
}

// запись модели: PresetSpec + uid для ключей списков; place — всегда объект (нейтральный = без места)
let uidSeq = 1
const nextUid = () => `s${uidSeq++}`

const clone = (v) => JSON.parse(JSON.stringify(v))

const NEUTRAL_PLACE = { pan: 0, width: 1 }

/** Пресет → модель редактора: рабочие копии рецепта (исходник не меняется). */
export function toModel(preset) {
  const p = preset || {}
  return {
    id: p.id || 0,
    builtin: !!p.builtin,
    name: p.name || '',
    note: p.note || '',
    specs: (p.specs || []).map((s) => ({
      uid: nextUid(),
      stems: [...(s.stems || [])],
      ...(s.engine ? { engine: clone(s.engine) } : {}),
      ...(s.chain ? { chain: s.chain } : {}),
      ...(s.params ? { params: { ...s.params } } : {}),
      ...(s.steps ? { steps: clone(s.steps) } : {}),
      db: s.db || 0,
      place: { ...NEUTRAL_PLACE, ...(s.place || {}) },
      levelDb: s.level_db != null ? s.level_db : null,
    })),
    final: clone(p.final || []),
    master: clone(p.master || []),
    targetLufs: p.target_lufs != null ? p.target_lufs : null,
    parts: clone(p.parts || []),
    referenceJobId: p.reference_job_id || 0,
  }
}

export function newModel() {
  return toModel({ name: '', note: '', specs: [] })
}

/** Модель → тело create/update (api.soundPresetCreate / Update): без id/slug/builtin/family — их
 *  назначает воркер. Нейтральное место не пишется, пустой level_db — не пишется; parts/final
 *  пробрасываются как есть. */
export function bodyFromModel(m) {
  const specs = (m.specs || []).map((s) => ({
    stems: [...(s.stems || [])],
    ...(s.engine ? { engine: clone(s.engine) } : {}),
    ...(s.chain ? { chain: s.chain } : {}),
    ...(s.params ? { params: { ...s.params } } : {}),
    ...(s.steps ? { steps: clone(s.steps) } : {}),
    db: s.db || 0,
    ...(s.place && (s.place.pan !== 0 || s.place.width !== 1) ? { place: { ...s.place } } : {}),
    ...(s.levelDb != null ? { level_db: s.levelDb } : {}),
  }))
  return {
    name: (m.name || '').trim(),
    note: (m.note || '').trim(),
    specs,
    final: clone(m.final || []),
    master: clone(m.master || []),
    target_lufs: m.targetLufs != null ? m.targetLufs : null,
    parts: clone(m.parts || []),
    reference_job_id: m.referenceJobId || 0,
  }
}

/** Копия пресета «Сохранить как свой»: рецепт целиком, имя с суффиксом (≤ 80 — длинное имя режем,
 *  суффикс «(копия)» сохраняем), без id/slug/builtin. */
export function dupBody(preset, suffix) {
  const name = String((preset && preset.name) || '')
  const suf = String(suffix || '')
  return {
    name: name.length + suf.length <= 80 ? name + suf : name.slice(0, 80 - suf.length) + suf,
    note: (preset && preset.note) || '',
    specs: clone((preset && preset.specs) || []),
    final: clone((preset && preset.final) || []),
    master: clone((preset && preset.master) || []),
    target_lufs: preset && preset.target_lufs != null ? preset.target_lufs : null,
    parts: clone((preset && preset.parts) || []),
    reference_job_id: (preset && preset.reference_job_id) || 0,
  }
}

/** Источник предпрослушивания записи: дорожка из stems, которая есть у эталона (available — Set имён),
 *  основные вперёд. Части барабанов у эталона нет — донастройка модели дорожек (RoFormer): blocked;
 *  дорожек нет вовсе — их ещё предстоит сделать (nostems). */
export function previewSource(stems, available) {
  const have = available instanceof Set ? available : new Set(available || [])
  const list = [...(stems || [])]
  const ordered = [...PREVIEW_ORDER, ...DRUM_PARTS, ...list.filter((s) => !PREVIEW_ORDER.includes(s) && !DRUM_PARTS.includes(s))]
  for (const s of ordered) {
    if (!list.includes(s)) continue
    if (have.has(s)) return { source: s }
  }
  const onlyParts = list.length > 0 && list.every((s) => DRUM_PARTS.includes(s))
  return { blocked: onlyParts ? 'roformer' : 'nostems' }
}

/** Чипы дорожек записи: дорожка есть — убрать, нет — добавить (пустым запись оставить нельзя). */
export function toggleStem(stems, name) {
  const cur = [...(stems || [])]
  const at = cur.indexOf(name)
  if (at >= 0) return cur.length > 1 ? cur.filter((s) => s !== name) : cur
  return [...cur, name]
}

/** «Ритм-секция набором» → записи пресета: части барабанов и бас, которые есть у эталона, сэмплами
 *  набора kit с обработкой treat (id из DRUM_TREATMENTS; 'dry' — без обработки). */
export function rhythmSpecs(available, treat, kit, locale = 'ru') {
  const names = [...(available instanceof Set ? available : available || [])]
  return rhythmSection(names, fxPresets, treat, locale, kit)
    // как обычные записи: свой uid (ключ списка и плеера — иначе ▶ одной части останавливал другую), место, уровень
    .map((r) => ({ uid: nextUid(), stems: [r.stem], engine: clone(r.chain), db: 0, levelDb: null, place: { ...NEUTRAL_PLACE } }))
}

/** Мастер пресета так же, как его применит Go (studio.presetMaster): цели нет — цепочка как есть; цель есть —
 *  ограничитель с этой целью (свой ограничитель с target_lufs 0 — получает цель, без него — добавляется в конец). */
export function presetMasterChain(chain, targetLufs) {
  const out = (chain || []).map((b) => ({ ...b }))
  if (targetLufs == null) return out
  // последний ограничитель — как в Go (там индекс перезаписывается на каждом)
  let lim = -1
  out.forEach((b, i) => { if (b.type === 'limiter') lim = i })
  if (lim < 0) return [...out, { type: 'limiter', target_lufs: targetLufs }]
  if (!out[lim].target_lufs) out[lim] = { ...out[lim], target_lufs: targetLufs }
  return out
}

/** Снимок редактора для «есть несохранённые правки?». Цепочки — в одной нормальной форме (с умолчаниями блоков):
 *  правка тонкой цепочки (sp.edit) учитывается, а раскрытие формы и запись мастера с умолчаниями правкой не считаются.
 *  id пресета не в счёт (появляется после сохранения нового); мастер — только из формы редактора (master). */
export function editorSnapshot(model, master) {
  const m = model || {}
  const norm = (sp) => (sp.edit ? toWorkerChain(sp.edit)
    : Array.isArray(sp.engine) ? toWorkerChain(fromWorkerChain(sp.engine, BLOCKS)) : sp.engine)
  return JSON.stringify({
    m: { ...m, id: undefined, master: undefined, specs: (m.specs || []).map((sp) => ({ ...sp, edit: undefined,
      engine: norm(sp) })) },
    master: master || [],
  })
}
