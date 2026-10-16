// Черновик студии — незаконченная работа над треком: записи реестра «Правки трека» (useInserts)
// и ожидающие рендеры кусков. Главный экземпляр — на воркере (/jobs/{id}/draft), localStorage —
// копия на ПК. Чистая логика без DOM: шаг записи, группировка по шагам, слияние копий, состояние.

// шаги студии, к которым относится правка, — в порядке страницы
export const EDIT_STEPS = ['edit', 'sound', 'voice', 'done']

// Шаг записи: явный step (эффект из «Обработки голоса» — voice) или по виду записи
export function editStep(it) {
  if (it && EDIT_STEPS.includes(it.step)) return it.step
  if (it.master) return 'done'
  if (it.mix) return 'sound'          // эффект на весь трек — шаг «Звук» (у «одного клика» явный step 'done')
  if (it.childId > 0) return 'edit'   // вклейка рендера («+ инструмент»)
  // «громкость дорожек» (заглушить, тише/громче) — приём шага «Править»
  if (!it.chain && !it.steps && !it.engine && !it.envelope && !it.place) return 'edit'
  return 'sound'
}

// Записи по шагам в порядке страницы; пустые шаги не выводятся, порядок внутри — как во входе
export function editsBySteps(list) {
  const by = Object.fromEntries(EDIT_STEPS.map((s) => [s, []]))
  for (const it of list || []) by[editStep(it)].push(it)
  return EDIT_STEPS.filter((s) => by[s].length).map((step) => ({ step, items: by[step] }))
}

const copy = (x) => JSON.parse(JSON.stringify(x))

// gen — изменения, которым нужен рендер: {specs: [приёмы плана], voice: {params, name} | null, instrumental}
export const isEmptyGen = (g) => !g || (!(g.specs || []).length && !g.voice && !g.instrumental)

// Состояние черновика трека: его записи, ожидающие рендеры (без поля parent — трек и так известен), gen и ход
// «применить всё» с рендером (applying: {job_id, title, …})
export function draftState(parentId, applied, pending, ts, gens, applyings) {
  const edits = copy((applied || {})[parentId] || [])
  const queue = (pending || []).filter((s) => String(s.parent) === String(parentId))
    .map(({ parent: _p, ...s }) => copy(s))
  const st = { v: 1, edits, queue, ts }
  const g = (gens || {})[parentId]
  if (!isEmptyGen(g)) st.gen = copy(g)
  const a = (applyings || {})[parentId]
  if (a) st.applying = copy(a)
  return st
}

export const isEmptyDraft = (st) => !st
  || (!(st.edits || []).length && !(st.queue || []).length && isEmptyGen(st.gen) && !st.applying)

// Слияние локальной копии с черновиками воркера — по каждому треку отдельно: побеждает более свежий
// по ts (локальный без ts — старше любого воркерного, равные — воркер). Трек с локальным ts — локальный
// и пустым: правки сбросили, пока воркер был недоступен, — свежее «пусто» должно победить старый черновик.
// Треки, где осталась локальная версия (новее или на воркере её нет), — в push: их надо отправить на
// воркер (пустые — удалить). Вход не мутируется.
export function mergeDrafts(local, localTs, remote) {
  const lApplied = (local && local.applied) || {}
  const lPending = (local && local.pending) || []
  const lGens = (local && local.gens) || {}
  const lApplying = (local && local.applying) || {}
  const lTs = localTs || {}
  // ключи — строкой, как у объекта applied: номер трека с воркера (число) и локальный ключ совпадают
  const key = (x) => String(x)
  const remoteBy = new Map((remote || []).map((d) => [key(d.job_id), d]))
  const localIds = new Set([
    ...Object.keys(lApplied).filter((k) => (lApplied[k] || []).length),
    ...lPending.map((s) => key(s.parent)),
    ...Object.keys(lTs),
    ...Object.keys(lGens).filter((k) => !isEmptyGen(lGens[k])),
    ...Object.keys(lApplying).filter((k) => lApplying[k]),
  ])
  const applied = {}
  const gens = {}
  const applying = {}
  let pending = []
  const ts = {}
  const push = []
  for (const id of new Set([...localIds, ...remoteBy.keys()])) {
    const d = remoteBy.get(id)
    const r = d && (d.state || {})
    const lt = lTs[id]
    const takeLocal = localIds.has(id) && (!r || (lt != null && lt > (r.ts || 0)))
    if (takeLocal) {
      if ((lApplied[id] || []).length) applied[id] = copy(lApplied[id])
      pending = pending.concat(lPending.filter((s) => key(s.parent) === id).map(copy))
      if (lt != null) ts[id] = lt
      if (!isEmptyGen(lGens[id])) gens[id] = copy(lGens[id])
      if (lApplying[id]) applying[id] = copy(lApplying[id])
      push.push(Number.isFinite(Number(id)) ? Number(id) : id)
    } else {
      if ((r.edits || []).length) applied[id] = copy(r.edits)
      pending = pending.concat((r.queue || []).map((s) => ({ ...copy(s), parent: d.job_id })))
      if (r.ts != null) ts[id] = r.ts
      if (!isEmptyGen(r.gen)) gens[id] = copy(r.gen)
      if (r.applying) applying[id] = copy(r.applying)
    }
  }
  return { applied, pending, gens, applying, ts, push }
}
