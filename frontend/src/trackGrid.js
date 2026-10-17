// Сетка тактов трека — одна на «Синт по аккордам» и «Перкуссию по сетке»: аккорды и части песни из плана,
// привязанные к долям звука (GET chord_grid). Разбор — один запрос на трек: второй блок берёт тот же ответ.
const cache = new Map()   // id трека → Promise сетки

/** Сетка трека {bpm, bars}; fresh — разобрать заново («разобрать заново» после новых дорожек или плана).
 *  Сбой (нет плана, нет долей) не запоминается: следующий вызов спросит воркер снова. */
export function chordGridOnce(api, jobId, { fresh = false } = {}) {
  if (fresh) cache.delete(jobId)
  if (!cache.has(jobId)) {
    const p = api.chordGrid(jobId)
    cache.set(jobId, p)
    p.catch(() => { if (cache.get(jobId) === p) cache.delete(jobId) })
  }
  return cache.get(jobId)
}
