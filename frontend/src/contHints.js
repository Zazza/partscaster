// «продолжить с места»: подсказки к приписке стиля. Подпись — на языке интерфейса
// (studio.cont.hint.<id>), модели уходит en: YuE2 понимает теги по-английски.
// Только то, чего нет в приёмах выше: инструменты, сбивка, нарастание, «жёстче»,
// аккорды мрачнее/светлее, темп и громкости дорожек — там, не здесь.
// Голосовую подачу (с придыханием, мощнее) не даём: приписки к подаче не слышны.
export const CONT_HINTS = [
  { id: 'energetic', group: 'energy', en: 'energetic, driving, upbeat' },
  { id: 'calm', group: 'energy', en: 'calm, mellow, laid-back' },
  { id: 'dense', group: 'energy', en: 'dense full arrangement, wall of sound' },
  { id: 'stripped', group: 'energy', en: 'stripped-down sparse arrangement, fewer instruments' },
  { id: 'climax', group: 'energy', en: 'climax, big finale, full band at maximum' },
  { id: 'fadeout', group: 'energy', en: 'winding down, fading out, gentle ending' },
  { id: 'nodrums', group: 'energy', en: 'no drums' },
  { id: 'cleangtr', group: 'sound', en: 'clean electric guitar, no distortion' },
  { id: 'acoustic', group: 'sound', en: 'acoustic guitar' },
  { id: 'gtrsolo', group: 'sound', en: 'guitar solo, lead guitar takes over' },
  { id: 'spacious', group: 'sound', en: 'spacious, lots of reverb, ambient' },
  { id: 'dry', group: 'sound', en: 'dry, tight, close-miked' },
  { id: 'lofi', group: 'sound', en: 'lo-fi, warm tape saturation' },
  { id: 'choir', group: 'sound', en: 'choir backing vocals' },
  { id: 'sad', group: 'mood', en: 'melancholic, sad' },
  { id: 'dreamy', group: 'mood', en: 'dreamy, ethereal' },
  { id: 'aggressive', group: 'mood', en: 'aggressive, raw, angry' },
  { id: 'epic', group: 'mood', en: 'epic, anthemic' },
  { id: 'blissful', group: 'mood', en: 'euphoric, blissful, uplifting' },
  { id: 'mysterious', group: 'mood', en: 'mysterious, haunting' },
  { id: 'dance', group: 'mood', en: 'danceable groove' },
]

export const CONT_HINT_GROUPS = ['energy', 'sound', 'mood']

/** Приписка к стилю: en выбранных подсказок в порядке списка, потом свой текст как есть. */
export function contStyleAdd(selectedIds, freeText) {
  const picked = CONT_HINTS.filter((h) => selectedIds.includes(h.id)).map((h) => h.en)
  const own = (freeText || '').trim()
  return [...picked, ...(own ? [own] : [])].join(', ')
}
