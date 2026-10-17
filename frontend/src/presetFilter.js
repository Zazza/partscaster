// Библиотека пресетов звука: порядок семей («Мои» — первыми), счётчики для фишек-фильтра и отбор
// по семье и поиску. Дерево — как presetTree (soundPresets.js): семьи → течения → пресеты; семья '' — свои.

/** «Мои» (семья '') — в начало, остальные — в прежнем порядке. */
export function minesFirst(tree) {
  const list = tree || []
  return [...list.filter((f) => f.family === ''), ...list.filter((f) => f.family !== '')]
}

const famCount = (f) => f.genres.reduce((n, g) => n + g.items.length, 0)

/** Фишки фильтра: «все» (family null) и каждая семья с числом пресетов, в порядке дерева. */
export function familyCounts(tree) {
  const list = tree || []
  const fams = list.map((f) => ({ family: f.family, label: f.label, count: famCount(f) }))
  return [{ family: null, label: 'all', count: fams.reduce((n, f) => n + f.count, 0) }, ...fams]
}

/** Отбор: family (null — все) и поиск q — подстрока без учёта регистра в названии, описании, течении,
 *  подписи семьи. Пустые течения и семьи выпадают; вход не меняется. */
export function filterPresetTree(tree, { q = '', family = null } = {}) {
  const needle = String(q || '').trim().toLowerCase()
  const hit = (...xs) => !needle || xs.some((x) => String(x || '').toLowerCase().includes(needle))
  const out = []
  for (const f of tree || []) {
    if (family != null && f.family !== family) continue
    const genres = []
    for (const g of f.genres) {
      const items = g.items.filter((p) => hit(p.name, p.note, g.genre, f.label))
      if (items.length) genres.push({ ...g, items })
    }
    if (genres.length) out.push({ ...f, genres })
  }
  return out
}
