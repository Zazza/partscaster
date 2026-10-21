// Слежение за вклейками инструментов (замены дорожек) — синглтон в module scope.
// Жить должно НЕ в компоненте студии: пока очередь гоняет 3-4 джобы,
// пользователь закрывает студию — компонент умирает, и готовые рендеры
// кусков никто не микширует. Этот сервис живёт всегда.
import { ref } from 'vue'
import { api } from '../api.js'
import { useI18n } from '../i18n/index.js'
import { sectionRequest, TRICK_INSTRUMENTS } from '../abcEdit.js'
import { renderJob } from '../requests.js'
import { clampDb, INSERT_DEFAULT_DB } from '../insertMix.js'
import { withStemPlace, withMaster } from '../mixDesk.js'
import { draftState, isEmptyDraft, isEmptyGen, genNeedsRender, genTimbre, mergeDrafts } from '../draftEdits.js'
import { draftSpecs, presetRecord, withPreset } from '../presetDraft.js'

const { t } = useI18n()
const KEY = 'yue_insert_queue'
// применённые вклейки по трекам: {parentId: [{childId, instId, from, to, lead,
// beat, db, stems, fadeIn, fadeOut, aligned, score}]} — из них трек
// пересобирается с чистого оригинала
const APPLIED_KEY = 'yue_insert_applied'
// время последней правки по трекам {parentId: мс} — по нему копия на ПК и черновик воркера сливаются
const TS_KEY = 'yue_insert_ts'
// что по каждому треку уже лежит на воркере (JSON правок и очереди): переживает перезапуск — иначе при
// старте без воркера все локальные треки казались бы изменёнными и затёрли бы его черновики
const SENT_KEY = 'yue_insert_sent'
// изменения с рендером по трекам {parentId: {specs, voice, instrumental}} и ход «применить всё» с рендером
// {parentId: {job_id, title} | {error, title} | {finalize: true, title}} — часть черновика трека
const GEN_KEY = 'yue_insert_gen'
const APPLYING_KEY = 'yue_insert_applying'
const pending = ref(load(KEY, []))
const applied = ref(load(APPLIED_KEY, {}))
const gens = ref(load(GEN_KEY, {}))
const applyings = ref(load(APPLYING_KEY, {}))
let ts = load(TS_KEY, {})
// ход «применить всё» двигаем только после стартового слияния с черновиками воркера: иначе запоздавший снимок
// воркера вернул бы уже завершённый ход (applying, gen, правки) — и итог собрался бы второй раз
let synced = false

function load(key, dflt) {
  try { return JSON.parse(localStorage.getItem(key) || 'null') || dflt } catch { return dflt }
}
function save() {
  markChanged()
  try {
    localStorage.setItem(KEY, JSON.stringify(pending.value))
    localStorage.setItem(APPLIED_KEY, JSON.stringify(applied.value))
    localStorage.setItem(GEN_KEY, JSON.stringify(gens.value))
    localStorage.setItem(APPLYING_KEY, JSON.stringify(applyings.value))
    localStorage.setItem(TS_KEY, JSON.stringify(ts))
    localStorage.setItem(SENT_KEY, JSON.stringify(sent))
  } catch { /* localStorage недоступен — живём в памяти до перезапуска */ }
  pushDirty()
}

// ---- черновик студии на воркере: главный экземпляр реестра (копия выше — на ПК) ----
// sent — что по каждому треку уже лежит на воркере (JSON правок и очереди без ts): изменившиеся треки
// получают новое время и уходят на воркер; так не нужно знать, какой вызов что поменял
const sent = load(SENT_KEY, {})
const dirty = new Set()
const stateOf = (id, at) => draftState(id, applied.value, pending.value, at, gens.value, applyings.value)
const body = (id) => {
  const st = stateOf(id, 0)
  return JSON.stringify([st.edits, st.queue, st.gen || null, st.applying || null])
}
function trackIds() {
  return new Set([...Object.keys(applied.value), ...pending.value.map((x) => String(x.parent)), ...Object.keys(sent),
    ...Object.keys(gens.value), ...Object.keys(applyings.value)])
}
function markChanged() {
  for (const id of trackIds()) {
    const b = body(id)
    if (b === (sent[id] ?? body.empty)) continue
    if (!dirty.has(id)) ts = { ...ts, [id]: Date.now() }
    dirty.add(id)
  }
}
body.empty = JSON.stringify([[], [], null, null])
// отправка по одной: черновики трека не должны обгонять друг друга
let pushing = Promise.resolve()
function pushDirty() {
  pushing = pushing.then(async () => {
    for (const id of [...dirty]) {
      const st = stateOf(id, ts[id] || Date.now())
      const b = body(id)
      try {
        const empty = isEmptyDraft(st)
        if (empty) await api.draftDelete(Number(id))
        else await api.draftPut(Number(id), st)
        // пока шла отправка, трек могли поменять ещё раз — тогда он остаётся в очереди отправки
        if (body(id) === b) {
          dirty.delete(id)
          if (empty) {
            // удалённому черновику не нужны ни время, ни отметка «отправлено»: иначе трек навсегда
            // остался бы «локальным» и при каждом старте слал бы удаление
            delete sent[id]
            ts = { ...ts }
            delete ts[id]
          } else sent[id] = b
        }
        persistSync()
      } catch (e) {
        // воркер недоступен (или старый, без черновиков): правки целы в localStorage, повтор — при следующем сохранении
        console.warn('черновик студии не отправлен на воркер', id, e)
      }
    }
  })
  return pushing
}
function persistSync() {
  try {
    localStorage.setItem(TS_KEY, JSON.stringify(ts))
    localStorage.setItem(SENT_KEY, JSON.stringify(sent))
  } catch { /* localStorage недоступен — отметки живут в памяти до перезапуска */ }
}
// старт: копия на ПК + черновики воркера, по каждому треку — более свежий; локальные новее — на воркер.
// Воркер недоступен — повтор позже: свежие черновики с другого ПК не должны затираться копией этого
const SYNC_RETRY_MS = 15000
async function syncFromWorker() {
  let remote
  try {
    remote = (await api.drafts()) || []
  } catch (e) {
    // старый воркер (нет /drafts — 404): черновиков там нет и не будет, правда — копия на ПК, ход можно двигать
    if (/\b404\b/.test(String(e))) {
      synced = true
      return
    }
    // нет связи — работаем по копии на ПК, слияние повторится
    console.warn('черновики студии с воркера не получены — повтор через', SYNC_RETRY_MS / 1000, 'с', e)
    setTimeout(syncFromWorker, SYNC_RETRY_MS)
    return
  }
  const m = mergeDrafts({ applied: applied.value, pending: pending.value, gens: gens.value,
    applying: applyings.value }, ts, remote)
  applied.value = m.applied
  pending.value = m.pending
  gens.value = m.gens
  applyings.value = m.applying
  ts = m.ts
  const pushIds = new Set(m.push.map(String))
  for (const d of remote) {
    const id = String(d.job_id)
    if (!pushIds.has(id)) sent[id] = body(id)
  }
  for (const id of pushIds) dirty.add(id)
  save()
  synced = true
}
syncFromWorker()

// specs: [{parent, childId, instId, from, to, lead, beat, db, stems, fadeIn, fadeOut, srcJob}]
function register(specs) {
  for (const sp of specs) notApplying(sp.parent)
  registerOwn(specs)
}
// без проверки «применить всё»: перенос вклеек на новую версию — шаг самого хода (carryTo)
function registerOwn(specs) {
  pending.value = [...pending.value, ...specs]
  save()
}
function byParent(parentId) {
  return pending.value.filter((s) => s.parent === parentId)
}
function appliedFor(parentId) {
  return applied.value[parentId] || []
}

// пересборки идут по одной: параллельные загрузки перетирали бы друг друга
let chain = Promise.resolve()
// сколько пересборок трека ждут или идут: пока их больше нуля, файл микса ещё старый —
// студия не даёт его слушать как «микс с правками» и делать из него версию
const building = ref({})
// пока идёт «применить всё», файл микса трека собирает итог — своя пересборка или превью перезаписали бы
// его между пересборкой итога и созданием трека (в итог попали бы чужие громкости)
function rebuild(parentId) {
  try { notApplying(parentId) } catch (e) { return Promise.reject(e) }
  return rebuildOwn(parentId)
}
// без проверки: пересборка, когда готов кусок вклейки (тик), — в т.ч. у новой версии, ждущей итога
function rebuildOwn(parentId) {
  return queued(parentId, () => doRebuild(parentId))
}
// «послушать громкости пресета до применения»: пересборка активных правок трека
// с временными записями громкости ВПЕРЕДИ (в реестр не попадают — трек не меняется,
// следующая пересборка из реестра вернёт микс без них). Идёт общей очередью пересборок
function previewSpecs(parentId, extra) {
  try { notApplying(parentId) } catch (e) { return Promise.reject(e) }
  return queued(parentId, async () => {
    const own = draftSpecs(appliedFor(parentId))
    const all = [...(extra || []), ...own]
    if (!all.length) return { empty: true }
    const r = await buildMix(parentId, all)
    if (r && r.variant) mixMetricsBy.value = { ...mixMetricsBy.value, [parentId]: r.variant.metrics || null }
    return r
  })
}
// «▶ стало» правки, которой ещё нет в списке: пересборка с ней (transform — список включённых правок →
// список с новой), в «Изменения трека» ничего не пишется
function previewWith(parentId, transform) {
  try { notApplying(parentId) } catch (e) { return Promise.reject(e) }
  return queued(parentId, async () => {
    const list = transform(appliedFor(parentId).filter((it) => !it.off))
    if (!list.length) return { empty: true }
    return buildMix(parentId, draftSpecs(list))
  })
}
// последняя удачная пересборка трека: записи (JSON) и файл микса. Все пересборки трека пишут один файл микса, поэтому
// «применить всё» берёт его вместо первой сборки, только если записи итога те же, что у последней пересборки
const lastBuilt = new Map()
async function buildMix(parentId, specs) {
  lastBuilt.delete(Number(parentId))
  let r
  try {
    r = await api.rebuildSections(parentId, specs)
  } catch (e) {
    throw markError(parentId, e)
  }
  const file = r && r.variant && r.variant.file
  if (file) lastBuilt.set(Number(parentId), { key: JSON.stringify(specs), file })
  markBuilt(parentId, r, specs)
  return r
}
// сбой сборки с отметкой правки (Go EditError: «… {edit:<id>}») — у этой записи поле error с причиной; ответ —
// ошибка для показа человеку, без служебной отметки
const EDIT_MARK = /\s*\{edit:([^}]+)\}/
function markError(parentId, e) {
  const msg = String((e && e.message) || e)
  const m = msg.match(EDIT_MARK)
  if (!m) return e
  const text = msg.replace(EDIT_MARK, '')
  const it = appliedFor(parentId).find((x) => String(x.childId) === m[1])
  if (it) {
    it.error = text
    applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId)] }
    save()
  }
  // Wails отклоняет промис строкой — так и оставить (String(Error) дал бы «Error: …»)
  return typeof e === 'string' ? text : new Error(text)
}
// удачная сборка: отметки сбоя сняты, предупреждения подготовки пресета — у записи-пресета, которая была в
// сборке (пресет, положенный во время сборки, чужих предупреждений не получает)
function markBuilt(parentId, r, specs) {
  const list = appliedFor(parentId)
  const built = new Set((specs || []).map((x) => x.id).filter(Boolean))
  for (const it of list) {
    delete it.error
    if (it.preset && built.has(String(it.childId))) {
      if (r && r.warnings && r.warnings.length) it.warnings = [...r.warnings]
      else delete it.warnings
    }
  }
  applied.value = { ...applied.value, [parentId]: [...list] }
  save()
}
// микс с правками устарел: записи черновика не те, что у последней удачной сборки
function mixStale(parentId) {
  const list = appliedFor(parentId).filter((it) => !it.off)
  if (!list.length) return false
  return !builtWith(parentId, draftSpecs(list))
}
// «▶ микс»: устарел — одна пересборка (общей очередью пересборок), иначе — готовый файл. Свежесть
// проверяется и в очереди: два «▶» подряд во время сборки не ставят вторую такую же сборку. Идёт «применить
// всё» — не собирать (минуты в общей очереди), играет последний микс
async function ensureMix(parentId) {
  const fresh = () => {
    const list = appliedFor(parentId).filter((it) => !it.off)
    return list.length ? builtWith(parentId, draftSpecs(list)) : ''
  }
  const file = fresh()
  if (file) return { cached: true, file }
  if (applying.has(Number(parentId)) || rendering(parentId)) return { cached: true, file: mixFile(parentId) }
  return queued(parentId, () => {
    const f = fresh()
    return f ? { cached: true, file: f } : doRebuild(parentId)
  })
}
// файл для «было» (трек со всеми правками черновика): устарел — сначала сборка; правок нет — null (оригинал)
async function currentMix(parentId) {
  if (!appliedFor(parentId).some((it) => !it.off)) return null
  const r = await ensureMix(parentId)
  return (r && (r.file || (r.variant && r.variant.file))) || mixFile(parentId)
}
// файл сборки-прослушивания, собранной ровно этими записями, или ''
function builtWith(parentId, specs) {
  const b = lastBuilt.get(Number(parentId))
  return b && b.key === JSON.stringify(specs) ? b.file : ''
}
function queued(parentId, fn) {
  building.value = { ...building.value, [parentId]: (building.value[parentId] || 0) + 1 }
  const run = chain.then(fn).finally(() => {
    building.value = { ...building.value, [parentId]: building.value[parentId] - 1 }
  })
  chain = run.catch(() => {})
  return run
}
const isBuilding = (parentId) => (building.value[parentId] || 0) > 0
async function doRebuild(parentId) {
  // выключенные записи остаются в реестре, но в пересборку не идут; активных нет — звучит оригинал
  const list = appliedFor(parentId).filter((it) => !it.off)
  if (!list.length) return { empty: true }
  const r = await buildMix(parentId, draftSpecs(list))
  // отчёт Go: встала ли вклейка по бочке или по плану (UI предупреждает).
  // Пишем в АКТУАЛЬНЫЙ реестр, а не в снимок до await: пока шла пересборка,
  // tick мог добавить готовую вклейку — снимок её бы затёр
  const cur = appliedFor(parentId)
  for (const rep of (r && r.inserts) || []) {
    const it = cur.find((x) => x.childId === rep.child_id)
    if (it) Object.assign(it, { aligned: rep.aligned, score: rep.score })
  }
  applied.value = { ...applied.value, [parentId]: [...cur] }
  save()
  if (r && r.variant) mixMetricsBy.value = { ...mixMetricsBy.value, [parentId]: r.variant.metrics || null }
  return r
}
// метрики последнего микса с правками (после мастера — его громкость и пик): строка «мастер: −14 LUFS…»
const mixMetricsBy = ref({})
const mixMetrics = (parentId) => mixMetricsBy.value[parentId] || null
function setMixMetrics(parentId, metrics) {
  mixMetricsBy.value = { ...mixMetricsBy.value, [parentId]: metrics || null }
}
// перенос правок на новую версию трека (пересборка): правки без рендера (эффекты, педали, движок,
// громкость, линии; childId ≤ 0) — сразу в реестр новой версии как есть (с off): в очереди тик не
// нашёл бы их джобу и потерял. Вклейки — через очередь: применённые с текущей громкостью,
// ещё рендерящиеся — как есть
function carryTo(fromParent, toParent, srcJob) {
  const all = appliedFor(fromParent).map(({ aligned: _a, score: _s, ...it }) => it)
  const own = all.filter((it) => !(it.childId > 0))
  if (own.length) {
    const have = new Set(appliedFor(toParent).map((it) => it.childId))
    const add = own.filter((it) => !have.has(it.childId)).map((it) => JSON.parse(JSON.stringify(it)))
    applied.value = { ...applied.value, [toParent]: [...appliedFor(toParent), ...add] }
  }
  const done = all.filter((it) => it.childId > 0)
  // вклейка может быть и в реестре, и в очереди (tick ждёт её пересборку):
  // берём одну — из реестра, там актуальная громкость
  const inRegistry = new Set(done.map((it) => it.childId))
  const waiting = byParent(fromParent).filter((s) => !inRegistry.has(s.childId) && s.childId > 0)
    .map(({ parent: _p, done: _d, dead: _x, ...it }) => it)
  registerOwn([...done, ...waiting].map((it) => ({ ...it, parent: toParent, srcJob })))
}

// файл свежего микса: Go называет его по последней активной вклейке реестра
function latestFile(parentId) {
  const list = appliedFor(parentId).filter((it) => !it.off && it.childId > 0)   // «заглушить» файла не именует
  return list.length ? `overdub-inst-${list[list.length - 1].childId}.flac` : null
}
// микс с правками для «▶ микс» студии: без вклеек (одни эффекты/громкости) Go называет его «-0»;
// активных записей нет — микса нет, звучит оригинал
function mixFile(parentId) {
  if (!appliedFor(parentId).some((it) => !it.off)) return null
  return latestFile(parentId) || 'overdub-inst-0.flac'
}

async function selectAlt(parentId, childId, altId) {
  notApplying(parentId)
  const it = appliedFor(parentId).find((x) => x.childId === childId)
  if (!it || !(it.alts || []).includes(altId)) return null
  it.childId = altId
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId)] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// «заглушить» (приём без рендера): сразу в реестр и пересборка. childId у
// таких записей — отрицательная метка времени (уникальна, в api уходит 0)
async function addMute(parentId, item) {
  return addMutes(parentId, [item])
}
// несколько «громкостей дорожек» разом — одна пересборка
let muteSeq = 0
async function addMutes(parentId, items) {
  notApplying(parentId)
  const added = items.map(({ instId, from, to, stems, db = -100, step = '' }) => ({
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId, from, to, lead: 0, beat: 0, db, stems,
    fadeIn: 0, fadeOut: 0, keepHighHz: 0,
    // шаг студии для списка «Изменения трека» («убрать» из «Дорожек» — sound); без него — по виду записи
    ...(step ? { step } : {}),
  }))
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), ...added] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// эффект на дорожку (стем) в окне: запись реестра как у «громкости дорожек»
// (childId < 0, в api уходит 0) + цепочка; копится вместе с вклейками
async function addStemFx(parentId, { stem, chain, params, from = 0, to = 0, step = '' }) {
  notApplying(parentId)
  const item = {
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'fx-' + chain, from, to, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, chain, params: { ...(params || {}) },
    // шаг студии для списка «Изменения трека» («Обработка голоса» — voice); без него — по виду записи
    ...(step ? { step } : {}),
  }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// доска педалей на дорожку в окне: одна запись реестра со списком шагов
// (эффекты по порядку, а не сумма отдельных); label — подпись в списке вставок
async function addStemPedals(parentId, { stem, steps, from = 0, to = 0, label = '', pedalPreset = '' }) {
  notApplying(parentId)
  const item = {
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'pedals', from, to, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, steps: (steps || []).map((s) => ({ ...s })), label,
    ...(pedalPreset ? { pedalPreset } : {}), // набор педалей — подпись берётся по id на языке показа
  }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// цепочка звукового движка воркера на дорожку в окне: одна запись реестра,
// считается на воркере при пересборке; label — подпись в списке вставок
// add — добавить кусок поверх трека, исходную дорожку не вычитать (синт-партия на «mix» — без разделения)
async function addStemEngine(parentId, { stem, chain, from = 0, to = 0, label = '', add = false, place = null }) {
  notApplying(parentId)
  const item = {
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'engine', from, to, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, engine: (chain || []).map((b) => ({ ...b })), label,
    ...(add ? { add: true } : {}),
    ...(add && place ? { place: { ...place } } : {}),
  }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// несколько цепочек движка разом («ритм-секция набором») — одна пересборка
async function addStemEngines(parentId, items) {
  notApplying(parentId)
  if (!(items || []).length) return null
  const added = items.map(({ stem, chain, from = 0, to = 0, label = '' }) => ({
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'engine', from, to, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, engine: (chain || []).map((b) => ({ ...b })), label,
  }))
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), ...added] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// линия громкости дорожки (по волне): одна запись на дорожку — повторная
// заменяет прежнюю, пустая — убирает; эффекты и вклейки не трогаются
async function addStemEnvelope(parentId, { stem, envelope }) {
  notApplying(parentId)
  const rest = appliedFor(parentId).filter((x) => !(x.envelope && (x.stems || [])[0] === stem))
  const item = envelope && envelope.length ? [{
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'env', from: 0, to: 0, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, envelope: envelope.map(({ t, db }) => ({ t, db })),
  }] : []
  applied.value = { ...applied.value, [parentId]: [...rest, ...item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// ---- эффекты на весь трек: записи mix ложатся на собранный микс (после правок дорожек, до мастера) ----
// «применить» эффекта/педалей/линии на весь трек и «один клик» копятся в черновике, а не версиями dsp-*
const mixItem = (extra) => ({
  childId: -(Date.now() * 100 + (muteSeq++ % 100)), from: 0, to: 0, lead: 0, beat: 0, db: 0,
  stems: ['mix'], fadeIn: 0, fadeOut: 0, keepHighHz: 0, mix: true, ...extra,
})
async function addMixFx(parentId, { chain, params, step = '', label = '', oneClick = '' }) {
  notApplying(parentId)
  const item = mixItem({ instId: 'fx-' + chain, chain, params: { ...(params || {}) },
    ...(step ? { step } : {}), ...(label ? { label } : {}), ...(oneClick ? { oneClick } : {}) })
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}
async function addMixPedals(parentId, { steps, label = '', pedalPreset = '' }) {
  notApplying(parentId)
  const item = mixItem({ instId: 'pedals', steps: (steps || []).map((s) => ({ ...s })), label, ...(pedalPreset ? { pedalPreset } : {}) })
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}
// линия громкости на весь трек: одна на трек — повторная заменяет, пустая убирает
async function setMixEnvelope(parentId, envelope) {
  notApplying(parentId)
  const rest = appliedFor(parentId).filter((x) => !(x.mix && x.envelope))
  const item = envelope && envelope.length
    ? [mixItem({ instId: 'env', envelope: envelope.map(({ t, db }) => ({ t, db })) })] : []
  applied.value = { ...applied.value, [parentId]: [...rest, ...item] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// выключить/вернуть запись: остаётся в реестре (и в localStorage), пересборка без неё
async function setOff(parentId, childId, off) {
  notApplying(parentId)
  const list = appliedFor(parentId)
  const it = list.find((x) => x.childId === childId)
  if (!it) return null
  it.off = !!off
  applied.value = { ...applied.value, [parentId]: [...list] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// удалить запись; ожидающая спека той же вклейки уходит из очереди — иначе готовый рендер
// вернул бы её в реестр. Записи не было — пересборки нет
async function remove(parentId, childId) {
  notApplying(parentId)
  const before = pending.value.length
  pending.value = pending.value.filter((s) => !(s.parent === parentId && s.childId === childId))
  const list = appliedFor(parentId)
  const rest = list.filter((x) => x.childId !== childId)
  if (rest.length === list.length) {
    if (pending.value.length !== before) save()
    return null
  }
  applied.value = { ...applied.value, [parentId]: rest }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// новая цепочка у записи движка: окно и дорожка прежние, подпись — новая, если дана
async function replaceEngine(parentId, childId, { chain, label } = {}) {
  notApplying(parentId)
  const list = appliedFor(parentId)
  const it = list.find((x) => x.childId === childId)
  if (!it || !it.engine) throw new Error(t('inserts.notEngine'))
  it.engine = (chain || []).map((b) => ({ ...b }))
  if (label) it.label = label
  applied.value = { ...applied.value, [parentId]: [...list] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// место дорожки в стерео (пульт): одна запись на дорожку, центр — запись убирается
async function setStemPlace(parentId, stem, place) {
  notApplying(parentId)
  applied.value = { ...applied.value, [parentId]: withStemPlace(appliedFor(parentId), stem, place) }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}
// место партии-добавления (синт, перкуссия) — у самой записи
async function setPlace(parentId, childId, place) {
  notApplying(parentId)
  const it = appliedFor(parentId).find((x) => x.childId === childId)
  if (!it) return null
  it.place = { ...place }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId)] }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}
// мастер трека: одна запись, новая заменяет прежнюю
async function setMaster(parentId, { chain, label = '' }) {
  notApplying(parentId)
  applied.value = { ...applied.value, [parentId]: withMaster(appliedFor(parentId), chain, label) }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

async function putPreset(parentId, preset, info) {
  notApplying(parentId)
  const rec = presetRecord(preset, info, -(Date.now() * 100 + (muteSeq++ % 100)))
  applied.value = { ...applied.value, [parentId]: withPreset(appliedFor(parentId), rec) }
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

async function setDb(parentId, childId, db) {
  notApplying(parentId)
  const it = appliedFor(parentId).find((x) => x.childId === childId)
  if (!it) return null
  it.db = clampDb(db)
  save()
  return null   // только запись: собирает «▶ микс» (ensureMix) и «применить всё»
}

// пока идёт «применить всё», правки трека не принимаются: итог собирается из списка на момент нажатия,
// а после успеха список очищается — правка, сделанная в это время, пропала бы молча
const applying = new Set()
// идёт ход «применить всё» с рендером: ждём рендер (job_id) или итог новой версии (finalize) — правки человека в это
// время не принимаются: итог собирается без них и стёр бы их (новый голос на версии, ждущей кусков, — потерялся бы)
const rendering = (parentId) => {
  const a = applyings.value[parentId]
  return !!(a && !a.error && (a.job_id || a.finalize))
}
function notApplying(parentId) {
  if (applying.has(Number(parentId)) || rendering(parentId)) throw new Error(t('inserts.applying'))
}

// ---- изменения с рендером (gen): голос, приёмы плана, «без голоса» ----
const genFor = (parentId) => gens.value[parentId] || null
const applyingFor = (parentId) => applyings.value[parentId] || null
// слияние полей; пустой gen убирается
function setGen(parentId, patch) {
  notApplying(parentId)
  const g = { specs: [], voice: null, instrumental: false, ...(gens.value[parentId] || {}), ...JSON.parse(JSON.stringify(patch || {})) }
  const rest = { ...gens.value }
  if (isEmptyGen(g)) delete rest[parentId]
  else rest[parentId] = g
  gens.value = rest
  save()
}
function setApplying(parentId, a) {
  const rest = { ...applyings.value }
  if (a) rest[parentId] = a
  else delete rest[parentId]
  applyings.value = rest
  save()
}
// ход «применить всё» с рендером: рендер готов — правки на новую версию, затем её итог; ошибка — причина видна,
// трек разблокирован; новая версия ещё ждёт куски вклеек — итог после них (finalize)
let advancing = false
async function advanceApplying() {
  if (advancing || !synced) return
  const live = Object.entries(applyings.value).filter(([, a]) => a && (a.finalize || (a.job_id && !a.error)))
  if (!live.length) return
  advancing = true
  try {
    const jobs = (await api.jobs()) || []
    for (const [pid, a] of live) {
      if (a.finalize) {
        // правки новой версии ждут свои рендеры кусков — итог, когда очередь пуста
        if (!byParent(Number(pid)).length && !isBuilding(pid)) await finalize(Number(pid), a.title)
        continue
      }
      const row = jobs.find((j) => Number(j.id) === Number(a.job_id))
      if (!row || ['error', 'canceled'].includes(row.status)) {
        setApplying(pid, { title: a.title, error: (row && (row.error || row.status)) || 'job not found' })
        continue
      }
      if (row.status !== 'done') {
        if (row.status !== a.status) setApplying(pid, { ...a, status: row.status })
        continue
      }
      if (a.timbre) {
        // рендер готов, следом — тембр песни: голос рендера переводится Seed-VC; правки и gen пока на исходном
        try {
          const vc = await api.voiceConvert(Number(a.job_id), { ref_job_id: a.timbre.ref_job_id })
          setApplying(pid, { job_id: vc, title: a.title, status: 'queued' })
        } catch (e) {
          setApplying(pid, { title: a.title, error: String((e && e.message) || e) })
        }
        continue
      }
      const newId = Number(a.job_id)
      const from = Number(pid)
      // правки трека — на новую версию (как у пересборки), у исходного трека — чисто
      carryTo(from, newId, from)
      const restGen = { ...gens.value }
      delete restGen[pid]
      gens.value = restGen
      setApplying(pid, null)
      dropTrack(from)
      await finalize(newId, a.title)
    }
  } catch (e) {
    // список джоб недоступен — следующий тик повторит
    console.warn('«применить всё»: ход не продвинут', e)
  } finally { advancing = false }
}
// итог новой версии: есть правки — «применить всё» на ней; нет — она и есть итог (переименовать)
// Отметка {finalize} на новой версии снимается только после успеха: упал шаг или приложение перезапустили во время
// него — тик повторит; ошибка шага — {error} на новой версии (видно в её «Готово»), итог можно собрать вручную
async function finalize(newId, title) {
  setApplying(newId, { finalize: true, title })
  if (byParent(newId).length) return   // куски вклеек ещё рендерятся — итог после них (тик)
  try {
    if (appliedFor(newId).some((it) => !it.off)) await applyAllOwn(newId, title)
    else {
      dropTrack(newId)   // выключенные записи не дают итога — трек без правок и есть итог
      await api.renameJob(newId, title)
    }
    setApplying(newId, null)
  } catch (e) {
    setApplying(newId, { title, error: String((e && e.message) || e) })
  }
}

// «применить всё»: один новый трек из всех включённых правок (Go: пересборка → трек → черновик удалён);
// после успеха реестр и ожидающие рендеры трека очищаются. Ошибка — реестр не тронут
function applyAll(parentId, title, genCtx) {
  notApplying(parentId)
  return applyAllOwn(parentId, title, genCtx)
}
// без проверки хода — для шага итога самого хода (finalize): отметка {finalize} стоит, пока он не закончится
function applyAllOwn(parentId, title, genCtx) {
  const g = genFor(parentId)
  const tb = genTimbre(g)
  if (genNeedsRender(g) && genCtx && genCtx.request) {
    // изменения с рендером: один рендер со всеми ними; ход — в черновике (переживает закрытие студии);
    // тембр песни ход помнит — переводом голоса после рендера
    applying.add(Number(parentId))
    return (async () => {
      const id = await api.submit(genCtx.request)
      setApplying(parentId, { job_id: id, title, status: 'queued', ...(tb ? { timbre: { ...tb } } : {}) })
      return id
    })().finally(() => applying.delete(Number(parentId)))
  }
  if (!genNeedsRender(g) && tb) {
    // только тембр: голос самого трека — тембром песни (Seed-VC), новая версия; дальше — как после рендера
    applying.add(Number(parentId))
    return (async () => {
      try {
        const id = await api.voiceConvert(Number(parentId), { ref_job_id: tb.ref_job_id })
        setApplying(parentId, { job_id: id, title, status: 'queued' })
        return id
      } catch (e) {
        setApplying(parentId, { title, error: String((e && e.message) || e) })
        throw e
      }
    })().finally(() => applying.delete(Number(parentId)))
  }
  applying.add(Number(parentId))
  // мимо общей цепочки пересборок: итог собирается минутами (в очереди воркера), и правки ДРУГИХ треков
  // не должны его ждать. Свои пересборки трека в это время не начнутся — правки отклоняются (applying),
  // а кнопка недоступна, пока пересборка трека идёт. Счётчик building — чтобы «▶ микс» ждал итога
  building.value = { ...building.value, [parentId]: (building.value[parentId] || 0) + 1 }
  return (async () => {
    const specs = draftSpecs(appliedFor(parentId))
    // итог пишет тот же файл микса (сборка, поправки сверки): после него прослушиванием этот файл уже не считается
    const mix = builtWith(parentId, specs)
    lastBuilt.delete(Number(parentId))
    const id = await api.applyEdits(parentId, specs, title, mix)
    dropTrack(parentId)
    return id
  })().catch((e) => { throw markError(parentId, e) }).finally(() => {
    applying.delete(Number(parentId))
    building.value = { ...building.value, [parentId]: building.value[parentId] - 1 }
  })
}
// «сбросить черновик»: все правки и ожидающие рендеры трека — прочь, черновик на воркере удаляется
// Новая версия, ждущая итога ({finalize}), — сброс разрешён: если кусок вклейки не собирается никогда, итог не
// придёт, и без этого трек остался бы заблокирован навсегда. Сброс снимает и отметку. Идущий рендер ({job_id}) и
// идущий шаг итога (applying) по-прежнему не сбрасываются
async function clearDraft(parentId) {
  const a = applyings.value[parentId]
  if (a && a.finalize && !a.error && !applying.has(Number(parentId))) setApplying(parentId, null)
  notApplying(parentId)
  dropTrack(parentId)
  await pushing
  // трек без правок черновика мог и не иметь — удаление всё равно отправляем: на воркере мог остаться чужой
  if (!dirty.has(String(parentId))) await api.draftDelete(parentId)
}
function dropTrack(parentId) {
  lastBuilt.delete(Number(parentId))
  const rest = { ...applied.value }
  delete rest[parentId]
  applied.value = rest
  const restGen = { ...gens.value }
  delete restGen[parentId]
  gens.value = restGen
  // причина прошлой неудачи «применить всё» уходит вместе с изменениями (идущий ход сюда не попадает: notApplying)
  if (applyings.value[parentId] && applyings.value[parentId].error) {
    const restA = { ...applyings.value }
    delete restA[parentId]
    applyings.value = restA
  }
  pending.value = pending.value.filter((s) => Number(s.parent) !== Number(parentId))
  save()
}

async function recreateChild(spec, src) {
  // рендер куска удалён из очереди — ставим такой же по плану исходной джобы
  const inst = TRICK_INSTRUMENTS.find((i) => i.id === spec.instId)
  if (!inst || !src) return null
  try {
    const abc = await api.jobAbcText(src.id, src.abc_file || 'score.abc')
    const req = sectionRequest(src, abc, inst, spec.from, spec.to ?? spec.from + 15)
    if (!req.abc.includes('|')) return null
    return api.submit(req)
  } catch { return null }  // воркер/план недоступны — спека уйдёт в dead ниже
}

// один проход очереди за раз: таймер и flush() (кнопка ▶ студии) делят его
let running = null
function tick() {
  if (!running) running = tickOnce().finally(() => { running = null })
  return running
}
// немедленно обработать готовые партии (идущий проход — дождаться и пройти ещё
// раз). Список джоб не получен — ошибка вызывающему: ▶ покажет её, а не сыграет
// прошлый микс без новой вклейки. Таймерный проход результат игнорирует.
async function flush() {
  if (running) await running
  const res = await tick()
  if (res && res.error) throw res.error
}

async function tickOnce() {
  if (!pending.value.length) return
  try {
    const jobs = await api.jobs()
    const find = (id) => (jobs || []).find((x) => x.id === id)
    for (const spec of pending.value) {
      let child = find(spec.childId)
      if (!child && spec.instId) {
        // у версии-варианта стиль/текст/сид — исходного трека (renderJob), иначе кусок рендерился бы без них
        const src = find(spec.srcJob)
        const childId = await recreateChild(spec, src && renderJob(src, jobs || []))
        if (childId) { spec.childId = childId; child = find(childId) }
      }
      if (!child) { spec.dead = true; continue }
      if (child.status === 'error' || child.status === 'canceled') { spec.dead = true; continue }
      if (child.status !== 'done') continue
      // спеку убрали (remove) пока тик ждал воркер — в реестр её не возвращаем
      if (!pending.value.includes(spec)) continue
      // повтор после сбоя пересборки: запись уже в реестре — не трогаем её
      // (громкость с ползунка и порядок сохраняются)
      const list = appliedFor(spec.parent)
      const prev = spec.replaces && list.find((x) => x.childId === spec.replaces)
      if (prev) {
        // «ещё вариант»: новый рендер становится текущим, прежние — в alts
        prev.alts = [...new Set([...(prev.alts || [prev.childId]), spec.childId])]
        prev.childId = spec.childId
        applied.value = { ...applied.value, [spec.parent]: [...list] }
      } else if (!list.some((x) => x.childId === spec.childId)) {
        applied.value = { ...applied.value, [spec.parent]: [...list, {
          childId: spec.childId, instId: spec.instId, from: spec.from,
          to: spec.to ?? spec.from + 15, lead: spec.lead || 0, beat: spec.beat || 0,
          db: spec.db ?? INSERT_DEFAULT_DB,
          stems: spec.stems || [], fadeIn: spec.fadeIn || 0, fadeOut: spec.fadeOut || 0,
          keepHighHz: spec.keepHighHz || 0, off: !!spec.off,
        }] }
      }
      spec.done = true   // только запись: микс соберёт «▶ микс» (ensureMix) или «применить всё»
    }
    pending.value = pending.value.filter((s) => !s.done && !s.dead)
    save()
  } catch (error) {
    return { error }   // jobs недоступны: таймер подождёт следующего тика, flush сообщит
  }
}

setInterval(tick, 3000)
setInterval(advanceApplying, 3000)

export function useInserts() {
  return { pending, applied, applyAll, ensureMix, currentMix, mixStale, clearDraft, genFor, setGen, applyingFor, register, byParent, appliedFor, rebuild, previewSpecs, previewWith, setDb, setOff, remove, replaceEngine, selectAlt, addMute, addMutes, addStemFx, addStemPedals, addMixFx, addMixPedals, setMixEnvelope, addStemEngine, addStemEngines, addStemEnvelope, setStemPlace, setPlace, setMaster, putPreset, mixMetrics, setMixMetrics, carryTo, flush, latestFile, mixFile, isBuilding }
}
