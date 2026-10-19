// «трек» клипа окна в плеере (Go: welcomeClipID) — не совпадает с джобами и кругом фраз
export const WELCOME_CLIP_ID = 2 ** 41

// Шаги окна «Что это»: первым — «Что это», последним — «Видеокарты», между ними шаги примеров
// из welcomeClips.json. Шаг без пар (заготовка) не показывается.
export function visibleSteps(data) {
  const steps = ((data && data.steps) || []).filter((s) => s && Array.isArray(s.pairs) && s.pairs.length > 0)
  return [{ id: 'about' }, ...steps, { id: 'gpu' }]
}

// Переключение «было/стало»: если клип играет — другая версия с той же секунды (не дальше конца
// клипа); не играет — null, только выбор версии без звука.
export function switchStart(playing, posSec, clipDurSec) {
  if (!playing) return null
  let pos = Math.max(0, Number(posSec) || 0)
  if (clipDurSec > 0) pos = Math.min(pos, clipDurSec)
  return pos
}
