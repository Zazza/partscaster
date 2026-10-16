// Поиск по пунктам селекта: пункт подходит, если в его подписи есть каждое
// слово запроса (подстрокой, без учёта регистра, «ё» = «е»). Пустой запрос —
// все пункты; порядок не меняется. Заголовки групп ({header: true}) — только над найденным.
const norm = (s) => String(s ?? '').toLowerCase().replace(/ё/g, 'е')

export function matchOptions(options, query) {
  const words = norm(query).split(/\s+/).filter(Boolean)
  if (!words.length) return [...options]
  const hit = (o) => {
    const label = norm(o.label) + (o.search ? ' ' + norm(o.search) : '')   // search — доп. текст поиска (семья, течение)
    return words.every((w) => label.includes(w))
  }
  // заголовок группы (header) остаётся, только если под ним (до следующего заголовка) что-то нашлось
  const out = []
  let head = null
  for (const o of options) {
    if (o.header) { head = o; continue }
    if (!hit(o)) continue
    if (head) { out.push(head); head = null }
    out.push(o)
  }
  return out
}
