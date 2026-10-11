// Слежение студии за запущенной джобой (кусок, новая версия, партия инструмента) — чистая логика без DOM.

/** Шаг опроса: строка джобы из списка (GET /jobs) поверх запомненного объекта. Строка списка не знает того, что
 *  запомнили при запуске (окно куска from/to), поэтому объект сливается, а не заменяется. Строки нет — null;
 *  done — джоба стала готовой на этом шаге (раньше не была). */
export function jobWatchStep(tracked, jobs) {
  if (!tracked || !tracked.id) return null
  const row = (jobs || []).find((x) => x.id === tracked.id)
  if (!row) return null
  return { next: { ...tracked, ...row }, row, done: row.status === 'done' && tracked.status !== 'done' }
}
