// Группы готовых цепочек (поле group в fxPresets.js): подписи и раскладка списка по группам для пульта
// дорожек и страницы «Инструменты». Чистая логика без DOM.

// Семейства инструментов — первый выбор на странице «Инструменты» и на пульте дорожек, по порядку
export const FAMILIES = [
  { id: 'guitar', ru: 'Гитара', en: 'Guitar' },
  { id: 'bass', ru: 'Бас', en: 'Bass' },
  { id: 'drums', ru: 'Барабаны', en: 'Drums' },
  { id: 'perc', ru: 'Перкуссия', en: 'Percussion' },
  { id: 'keys', ru: 'Клавиши и синтезаторы', en: 'Keys and synths' },
  { id: 'orch', ru: 'Оркестр', en: 'Orchestra' },
  { id: 'fx', ru: 'Обработка', en: 'Processing' },
]

// Группа = подвид внутри семейства (family); ru/en — подпись подвида: чем он отличается от соседних.
// Порядок ключей — порядок подвидов в списке
export const PRESET_GROUPS = {
  'guitar-clean': { family: 'guitar', ru: 'Чистые', en: 'Clean' },
  'guitar-drive': { family: 'guitar', ru: 'Перегруз', en: 'Drive' },
  'guitar-space': { family: 'guitar', ru: 'Пространство и эффекты', en: 'Space and effects' },
  'guitar-amp': { family: 'guitar', ru: 'Через усилитель (нужен захват NAM)', en: 'Through an amp (needs a NAM capture)' },
  'bass-tone': { family: 'bass', ru: 'Другой звук тех же нот', en: 'Different tone, same notes' },
  'bass-genre': { family: 'bass', ru: 'Звук под жанр', en: 'Tone by genre' },
  'bass-kit': { family: 'bass', ru: 'Ноты заменяются сэмплами', en: 'Notes replaced with samples' },
  'bass-synth': { family: 'bass', ru: 'Синтезатор — новая партия по аккордам', en: 'Synth — a new part on the chords' },
  'drums-osdk': { family: 'drums', ru: 'Живой набор', en: 'Real kit' },
  'drums-tr808': { family: 'drums', ru: 'Драм-машина 808', en: 'Drum machine 808' },
  'drums-tr909': { family: 'drums', ru: 'Драм-машина 909', en: 'Drum machine 909' },
  'drums-linn': { family: 'drums', ru: 'Сэмплерная машина 80-х (Linn)', en: '80s sampling machine (Linn)' },
  'drums-cr78': { family: 'drums', ru: 'Ранняя ритм-машина (CR-78)', en: 'Early rhythm machine (CR-78)' },
  'drums-simmons': { family: 'drums', ru: 'Электронные барабаны 80-х (Simmons)', en: '80s electronic drums (Simmons)' },
  // прежняя общая группа наборов: её хранят свои инструменты, сохранённые до разбивки по наборам
  'drum-kits': { family: 'drums', ru: 'Наборы', en: 'Kits' },
  perc: { family: 'perc', ru: 'Рисунки по сетке', en: 'Patterns on the grid' },
  'synth-keys': { family: 'keys', ru: 'Фортепиано и электропиано', en: 'Pianos and electric pianos' },
  'synth-organ': { family: 'keys', ru: 'Органы', en: 'Organs' },
  'synth-pad': { family: 'keys', ru: 'Пэды и струнные', en: 'Pads and strings' },
  'synth-lead': { family: 'keys', ru: 'Лиды', en: 'Leads' },
  'synth-toy': { family: 'keys', ru: 'Игрушки и колокольчики', en: 'Toys and bells' },
  'orch-strings': { family: 'orch', ru: 'Струнные', en: 'Strings' },
  'orch-winds': { family: 'orch', ru: 'Деревянные духовые', en: 'Woodwinds' },
  'orch-brass': { family: 'orch', ru: 'Медь', en: 'Brass' },
  'orch-mallets': { family: 'orch', ru: 'Колокольчики и маримба', en: 'Mallets' },
  'orch-bass': { family: 'orch', ru: 'Бас → оркестр', en: 'Bass → orchestra' },
  'orch-lead': { family: 'orch', ru: 'Мелодия → оркестр', en: 'Melody → orchestra' },
  mix: { family: 'fx', ru: 'Дорожки: голос, барабаны, синт', en: 'Tracks: vocals, drums, synth' },
  master: { family: 'fx', ru: 'Мастер', en: 'Master' },
}

const tr = (l, locale) => (l ? l[locale] || l.ru || '' : '')
const familyLabel = (id, locale) => tr(FAMILIES.find((f) => f.id === id), locale)

/** Список по группам: [{group, label «семейство · подвид», items}] в порядке первого появления группы, внутри — исходный порядок;
 *  пресет без group — в группе '' с пустой подписью. */
export function groupPresets(list, locale = 'ru') {
  const out = []
  const at = new Map()
  for (const p of list || []) {
    const g = p.group || ''
    if (!at.has(g)) {
      const l = PRESET_GROUPS[g]
      at.set(g, out.length)
      out.push({ group: g, label: l ? `${familyLabel(l.family, locale)} · ${tr(l, locale)}` : '', items: [] })
    }
    out[at.get(g)].items.push(p)
  }
  return out
}

// itemOptions — пункты выбранной группы (элемент groupPresets): {value: id, label, title} на языке locale
export function itemOptions(group, locale = 'ru') {
  const tr = (l) => (l ? l[locale] || l.ru || '' : '')
  return ((group && group.items) || []).map((p) => ({ value: p.id, label: tr(p.name), title: tr(p.note) }))
}

// Три выбора «семейство → подвид → инструмент»: [{family, label, subs: [{group, label, items}]}].
// Семейства — по порядку FAMILIES (пустые не выводятся), подвиды — по порядку PRESET_GROUPS (свои неизвестные —
// за ними по первому появлению), пункты — исходный порядок;
// группа вне PRESET_GROUPS (или без группы) — семейство 'other' последним с подписью otherLabel.
export function instrumentTree(list, locale = 'ru', otherLabel = '') {
  const fams = new Map()
  for (const p of list || []) {
    const g = p.group || ''
    const known = PRESET_GROUPS[g]
    const fam = known ? known.family : 'other'
    if (!fams.has(fam)) fams.set(fam, new Map())
    const subs = fams.get(fam)
    if (!subs.has(g)) subs.set(g, { group: g, label: known ? tr(known, locale) : '', items: [] })
    subs.get(g).items.push(p)
  }
  const order = Object.keys(PRESET_GROUPS)
  const rank = (g) => (order.includes(g) ? order.indexOf(g) : order.length)
  // sort устойчивая: неизвестные группы сохраняют порядок появления
  const node = (fam, label) => ({ family: fam, label, subs: [...fams.get(fam).values()].sort((a, b) => rank(a.group) - rank(b.group)) })
  const out = FAMILIES.filter((f) => fams.has(f.id)).map((f) => node(f.id, tr(f, locale)))
  if (fams.has('other')) out.push(node('other', otherLabel))
  return out
}

export function familyOptions(tree) {
  return (tree || []).map((f) => ({ value: f.family, label: f.label }))
}

// подвиды семейства node (элемент instrumentTree)
export function subOptions(node) {
  return ((node && node.subs) || []).map((s) => ({ value: s.group, label: s.label }))
}

// pathOf — где лежит инструмент id: {family, group}; не найден — первое семейство и его первый подвид
export function pathOf(tree, id) {
  const list = tree || []
  for (const f of list) {
    for (const s of f.subs) if (s.items.some((p) => p.id === id)) return { family: f.family, group: s.group }
  }
  return list.length ? { family: list[0].family, group: list[0].subs[0]?.group ?? '' } : { family: '', group: '' }
}
