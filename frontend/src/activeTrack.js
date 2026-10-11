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

/** Ручной выбор цели правок («на что» у громкости, эффектов, педалей): stem — какую дорожку сделать активной
 *  (null — активную не менять), target — цель правок. Трек ещё не разделён (дорожек нет) — цель держится как
 *  выбрана, активной дорожки нет (дорожку на эффект Go отделит сам); иначе активной становится дорожка цели
 *  (stemForTarget), цель — по ней (targetFor). */
export function targetChoice(target, stems, current = '') {
  if (!(stems || []).length) return { stem: null, target }
  const stem = stemForTarget(target, stems, current)
  return { stem: stem !== current ? stem : null, target: targetFor(stem) }
}
