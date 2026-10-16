// Черновик студии — незаконченная работа над треком: записи реестра «Правки трека» (useInserts)
// и ожидающие рендеры кусков. Главный экземпляр — на воркере (/jobs/{id}/draft), localStorage —
// копия на ПК. Чистая логика без DOM: шаг записи, группировка по шагам, слияние копий, состояние.

// шаги студии, к которым относится правка, — в порядке страницы
export const EDIT_STEPS = ['edit', 'sound', 'voice', 'done']

// Шаг записи: явный step (эффект из «Обработки голоса» — voice) или по виду записи
export function editStep(it) {
  if (it && EDIT_STEPS.includes(it.step)) return it.step
  if (it.master) return 'done'
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

// Состояние черновика трека: его записи и его ожидающие рендеры (без поля parent — трек и так известен)
export function draftState(parentId, applied, pending, ts) {
  const edits = copy((applied || {})[parentId] || [])
  const queue = (pending || []).filter((s) => Number(s.parent) === Number(parentId))
    .map(({ parent: _p, ...s }) => copy(s))
  return { v: 1, edits, queue, ts }
}

export const isEmptyDraft = (st) => !st || (!(st.edits || []).length && !(st.queue || []).length)

// Слияние локальной копии с черновиками воркера — по каждому треку отдельно: побеждает более свежий
// по ts (локальный без ts — старше любого воркерного, равные — воркер). Трек с локальным ts — локальный
// и пустым: правки сбросили, пока воркер был недоступен, — свежее «пусто» должно победить старый черновик.
// Треки, где осталась локальная версия (новее или на воркере её нет), — в push: их надо отправить на
// воркер (пустые — удалить). Вход не мутируется.
export function mergeDrafts(local, localTs, remote) {
  const lApplied = (local && local.applied) || {}
  const lPending = (local && local.pending) || []
  const lTs = localTs || {}
  const remoteBy = new Map((remote || []).map((d) => [Number(d.job_id), d.state || {}]))
  const localIds = new Set([
    ...Object.keys(lApplied).filter((k) => (lApplied[k] || []).length).map(Number),
    ...lPending.map((s) => Number(s.parent)),
    ...Object.keys(lTs).map(Number),
  ])
  const applied = {}
  let pending = []
  const ts = {}
  const push = []
  for (const id of new Set([...localIds, ...remoteBy.keys()])) {
    const r = remoteBy.get(id)
    const lt = lTs[id]
    const takeLocal = localIds.has(id) && (!r || (lt != null && lt > (r.ts || 0)))
    if (takeLocal) {
      if ((lApplied[id] || []).length) applied[id] = copy(lApplied[id])
      pending = pending.concat(lPending.filter((s) => Number(s.parent) === id).map(copy))
      if (lt != null) ts[id] = lt
      push.push(id)
    } else {
      if ((r.edits || []).length) applied[id] = copy(r.edits)
      pending = pending.concat((r.queue || []).map((s) => ({ ...copy(s), parent: id })))
      if (r.ts != null) ts[id] = r.ts
    }
  }
  return { applied, pending, ts, push }
}
