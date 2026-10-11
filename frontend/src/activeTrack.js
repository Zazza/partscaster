// Активная дорожка студии: один выбор «с какой дорожкой работаю» вместо пяти списков (волна, линия громкости,
// эффекты, педали, пульт). Чистая логика без DOM.

const DRUM_PARTS = ['kick', 'snare', 'toms', 'hh', 'ride', 'crash']
const TARGETS = ['vocals', 'drums', 'bass', 'other', 'guitar', 'piano']

/** Цель правок дорожки (линия громкости, эффекты): часть барабанов → барабаны; '' / неизвестная → весь трек ''. */
export function targetFor(stem) {
  if (DRUM_PARTS.includes(stem)) return 'drums'
  return TARGETS.includes(stem) ? stem : ''
}

/** Какую дорожку сделать активной, когда цель правок выбрали вручную: текущая уже даёт эту цель (kick при drums) —
 *  она же; есть дорожка с именем цели — она; иначе — весь трек ''. */
export function stemForTarget(target, stems, current = '') {
  if (!target) return ''
  if (current && targetFor(current) === target) return current
  return (stems || []).includes(target) ? target : ''
}
