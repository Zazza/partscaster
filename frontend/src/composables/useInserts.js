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
import { draftState, isEmptyDraft, mergeDrafts } from '../draftEdits.js'

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
const pending = ref(load(KEY, []))
const applied = ref(load(APPLIED_KEY, {}))
let ts = load(TS_KEY, {})

function load(key, dflt) {
  try { return JSON.parse(localStorage.getItem(key) || 'null') || dflt } catch { return dflt }
}
function save() {
  markChanged()
  try {
    localStorage.setItem(KEY, JSON.stringify(pending.value))
    localStorage.setItem(APPLIED_KEY, JSON.stringify(applied.value))
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
const body = (id) => {
  const st = draftState(id, applied.value, pending.value, 0)
  return JSON.stringify([st.edits, st.queue])
}
function trackIds() {
  return new Set([...Object.keys(applied.value), ...pending.value.map((x) => String(x.parent)), ...Object.keys(sent)])
}
function markChanged() {
  for (const id of trackIds()) {
    const b = body(id)
    if (b === (sent[id] ?? body.empty)) continue
    if (!dirty.has(id)) ts = { ...ts, [id]: Date.now() }
    dirty.add(id)
  }
}
body.empty = JSON.stringify([[], []])
// отправка по одной: черновики трека не должны обгонять друг друга
let pushing = Promise.resolve()
function pushDirty() {
  pushing = pushing.then(async () => {
    for (const id of [...dirty]) {
      const st = draftState(id, applied.value, pending.value, ts[id] || Date.now())
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
    // старый воркер или нет связи — работаем по копии на ПК, слияние повторится
    console.warn('черновики студии с воркера не получены — повтор через', SYNC_RETRY_MS / 1000, 'с', e)
    setTimeout(syncFromWorker, SYNC_RETRY_MS)
    return
  }
  const m = mergeDrafts({ applied: applied.value, pending: pending.value }, ts, remote)
  applied.value = m.applied
  pending.value = m.pending
  ts = m.ts
  const pushIds = new Set(m.push.map(String))
  for (const d of remote) {
    const id = String(d.job_id)
    if (!pushIds.has(id)) sent[id] = body(id)
  }
  for (const id of pushIds) dirty.add(id)
  save()
}
syncFromWorker()

// specs: [{parent, childId, instId, from, to, lead, beat, db, stems, fadeIn, fadeOut, srcJob}]
function register(specs) {
  for (const sp of specs) notApplying(sp.parent)
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
  return queued(parentId, () => doRebuild(parentId))
}
// «послушать громкости пресета до применения»: пересборка активных правок трека
// с временными записями громкости ВПЕРЕДИ (в реестр не попадают — трек не меняется,
// следующая пересборка из реестра вернёт микс без них). Идёт общей очередью пересборок
function previewSpecs(parentId, extra) {
  try { notApplying(parentId) } catch (e) { return Promise.reject(e) }
  return queued(parentId, async () => {
    const own = appliedFor(parentId).filter((it) => !it.off).map(specOf)
    const all = [...(extra || []), ...own]
    if (!all.length) return { empty: true }
    const r = await api.rebuildSections(parentId, all)
    if (r && r.variant) mixMetricsBy.value = { ...mixMetricsBy.value, [parentId]: r.variant.metrics || null }
    return r
  })
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
// запись реестра правок → спецификация пересборки
function specOf(it) {
  return {
    child_id: it.childId > 0 ? it.childId : 0, from: it.from, to: it.to,
    // у «громкости дорожек» (childId ≤ 0) db без зажима: −100 — заглушить
    lead: it.lead || 0, beat_sec: it.beat || 0, db: it.childId > 0 ? clampDb(it.db) : it.db,
    stems: it.stems || [], fade_in: it.fadeIn || 0, fade_out: it.fadeOut || 0,
    keep_high_hz: it.keepHighHz || 0,
    // эффект на дорожку: цепочка и её крутилки
    ...(it.chain ? { chain: it.chain, params: it.params || {} } : {}),
    // доска педалей на дорожку: цепочка эффектов по порядку
    ...(it.steps ? { steps: it.steps } : {}),
    // линия громкости дорожки (по волне): точки {t, db} по всему треку
    ...(it.envelope ? { envelope: it.envelope } : {}),
    // цепочка звукового движка воркера: JSON-блоки как у fx_apply
    ...(it.engine ? { engine: it.engine } : {}),
    // добавление поверх трека (синт): исходная дорожка не вычитается
    ...(it.add ? { add: true } : {}),
    // место в стерео: у партии-добавления — её место, своей записью — место дорожки
    ...(it.place ? { place: it.place } : {}),
    // мастер: цепочка движка на весь собранный микс на воркере
    ...(it.master ? { master: true } : {}),
    // эффект на весь собранный микс (до мастера): цепочка, педали или линия громкости
    ...(it.mix ? { mix: true } : {}),
  }
}
async function doRebuild(parentId) {
  // выключенные записи остаются в реестре, но в пересборку не идут; активных нет — звучит оригинал
  const list = appliedFor(parentId).filter((it) => !it.off)
  if (!list.length) return { empty: true }
  const r = await api.rebuildSections(parentId, list.map(specOf))
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
  register([...done, ...waiting].map((it) => ({ ...it, parent: toParent, srcJob })))
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
  return rebuild(parentId)
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
  const added = items.map(({ instId, from, to, stems, db = -100 }) => ({
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId, from, to, lead: 0, beat: 0, db, stems,
    fadeIn: 0, fadeOut: 0, keepHighHz: 0,
  }))
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), ...added] }
  save()
  return rebuild(parentId)
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
  return rebuild(parentId)
}

// доска педалей на дорожку в окне: одна запись реестра со списком шагов
// (эффекты по порядку, а не сумма отдельных); label — подпись в списке вставок
async function addStemPedals(parentId, { stem, steps, from = 0, to = 0, label = '' }) {
  notApplying(parentId)
  const item = {
    childId: -(Date.now() * 100 + (muteSeq++ % 100)), instId: 'pedals', from, to, lead: 0, beat: 0, db: 0,
    stems: [stem], fadeIn: 0, fadeOut: 0, keepHighHz: 0, steps: (steps || []).map((s) => ({ ...s })), label,
  }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return rebuild(parentId)
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
  return rebuild(parentId)
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
  return rebuild(parentId)
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
  return rebuild(parentId)
}

// ---- эффекты на весь трек: записи mix ложатся на собранный микс (после правок дорожек, до мастера) ----
// «применить» эффекта/педалей/линии на весь трек и «один клик» копятся в черновике, а не версиями dsp-*
const mixItem = (extra) => ({
  childId: -(Date.now() * 100 + (muteSeq++ % 100)), from: 0, to: 0, lead: 0, beat: 0, db: 0,
  stems: ['mix'], fadeIn: 0, fadeOut: 0, keepHighHz: 0, mix: true, ...extra,
})
async function addMixFx(parentId, { chain, params, step = '', label = '' }) {
  notApplying(parentId)
  const item = mixItem({ instId: 'fx-' + chain, chain, params: { ...(params || {}) },
    ...(step ? { step } : {}), ...(label ? { label } : {}) })
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return rebuild(parentId)
}
async function addMixPedals(parentId, { steps, label = '' }) {
  notApplying(parentId)
  const item = mixItem({ instId: 'pedals', steps: (steps || []).map((s) => ({ ...s })), label })
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId), item] }
  save()
  return rebuild(parentId)
}
// линия громкости на весь трек: одна на трек — повторная заменяет, пустая убирает
async function setMixEnvelope(parentId, envelope) {
  notApplying(parentId)
  const rest = appliedFor(parentId).filter((x) => !(x.mix && x.envelope))
  const item = envelope && envelope.length
    ? [mixItem({ instId: 'env', envelope: envelope.map(({ t, db }) => ({ t, db })) })] : []
  applied.value = { ...applied.value, [parentId]: [...rest, ...item] }
  save()
  return rebuild(parentId)
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
  return rebuild(parentId)
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
  return rebuild(parentId)
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
  return rebuild(parentId)
}

// место дорожки в стерео (пульт): одна запись на дорожку, центр — запись убирается
async function setStemPlace(parentId, stem, place) {
  notApplying(parentId)
  applied.value = { ...applied.value, [parentId]: withStemPlace(appliedFor(parentId), stem, place) }
  save()
  return rebuild(parentId)
}
// место партии-добавления (синт, перкуссия) — у самой записи
async function setPlace(parentId, childId, place) {
  notApplying(parentId)
  const it = appliedFor(parentId).find((x) => x.childId === childId)
  if (!it) return null
  it.place = { ...place }
  applied.value = { ...applied.value, [parentId]: [...appliedFor(parentId)] }
  save()
  return rebuild(parentId)
}
// мастер трека: одна запись, новая заменяет прежнюю
async function setMaster(parentId, { chain, label = '' }) {
  notApplying(parentId)
  applied.value = { ...applied.value, [parentId]: withMaster(appliedFor(parentId), chain, label) }
  save()
  return rebuild(parentId)
}

async function setDb(parentId, childId, db) {
  notApplying(parentId)
  const it = appliedFor(parentId).find((x) => x.childId === childId)
  if (!it) return null
  it.db = clampDb(db)
  save()
  return rebuild(parentId)
}

// пока идёт «применить всё», правки трека не принимаются: итог собирается из списка на момент нажатия,
// а после успеха список очищается — правка, сделанная в это время, пропала бы молча
const applying = new Set()
function notApplying(parentId) {
  if (applying.has(Number(parentId))) throw new Error(t('inserts.applying'))
}

// «применить всё»: один новый трек из всех включённых правок (Go: пересборка → трек → черновик удалён);
// после успеха реестр и ожидающие рендеры трека очищаются. Ошибка — реестр не тронут
function applyAll(parentId, title) {
  notApplying(parentId)
  applying.add(Number(parentId))
  // мимо общей цепочки пересборок: итог собирается минутами (в очереди воркера), и правки ДРУГИХ треков
  // не должны его ждать. Свои пересборки трека в это время не начнутся — правки отклоняются (applying),
  // а кнопка недоступна, пока пересборка трека идёт. Счётчик building — чтобы «▶ микс» ждал итога
  building.value = { ...building.value, [parentId]: (building.value[parentId] || 0) + 1 }
  return (async () => {
    const specs = appliedFor(parentId).filter((it) => !it.off).map(specOf)
    const id = await api.applyEdits(parentId, specs, title)
    dropTrack(parentId)
    return id
  })().finally(() => {
    applying.delete(Number(parentId))
    building.value = { ...building.value, [parentId]: building.value[parentId] - 1 }
  })
}
// «сбросить черновик»: все правки и ожидающие рендеры трека — прочь, черновик на воркере удаляется
async function clearDraft(parentId) {
  notApplying(parentId)
  dropTrack(parentId)
  await pushing
  // трек без правок черновика мог и не иметь — удаление всё равно отправляем: на воркере мог остаться чужой
  if (!dirty.has(String(parentId))) await api.draftDelete(parentId)
}
function dropTrack(parentId) {
  const rest = { ...applied.value }
  delete rest[parentId]
  applied.value = rest
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
      try {
        await rebuild(spec.parent)
        spec.done = true
      } catch { /* сеть/воркер моргнул — вклейка уже в реестре, пересборка на следующем тике */ }
    }
    pending.value = pending.value.filter((s) => !s.done && !s.dead)
    save()
  } catch (error) {
    return { error }   // jobs недоступны: таймер подождёт следующего тика, flush сообщит
  }
}

setInterval(tick, 3000)

export function useInserts() {
  return { pending, applied, applyAll, clearDraft, register, byParent, appliedFor, rebuild, previewSpecs, setDb, setOff, remove, replaceEngine, selectAlt, addMute, addMutes, addStemFx, addStemPedals, addMixFx, addMixPedals, setMixEnvelope, addStemEngine, addStemEngines, addStemEnvelope, setStemPlace, setPlace, setMaster, mixMetrics, setMixMetrics, carryTo, flush, latestFile, mixFile, isBuilding }
}
