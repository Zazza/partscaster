// Сколько вариантов трека ставить кнопкой «Варианты»: целое 2..5; мусор (пусто, не число) —
// по умолчанию 3. Значение приходит из браузерного хранилища строкой.
export const FAN_MIN = 2
export const FAN_MAX = 5
export const FAN_DEFAULT = 3

export function fanCount(v) {
  if (v == null || (typeof v === 'string' && !v.trim())) return FAN_DEFAULT
  const n = Number(v)
  if (!Number.isFinite(n)) return FAN_DEFAULT
  return Math.min(FAN_MAX, Math.max(FAN_MIN, Math.round(n)))
}
