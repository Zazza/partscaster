// Инструментальная стойка: карточки по группам, выбор разворачивается
// в текст стиля автоматически — словесная кухня спрятана.
// rackSel: [{ id, effect }] — входит в параметры генерации.

export const rackGroups = [
  {
    id: 'rhythm', name: 'Ритм-секция', nameEn: 'Rhythm section',
    items: [
      { id: 'drums-tight', name: 'Барабаны: сухой панч', nameEn: 'Drums: dry punch', phrasing: 'tight dry punchy drums, close-miked' },
      { id: 'drums-room', name: 'Барабаны: комнатный', nameEn: 'Drums: roomy', phrasing: 'roomy live drums, natural bleed' },
      { id: 'drums-machine', name: 'Драм-машина', nameEn: 'Drum machine', phrasing: 'cheap drum machine, rigid electronic beat' },
      { id: 'bass-drive', name: 'Бас: напористый', nameEn: 'Bass: driving', phrasing: 'driving picked bass, forward in mix' },
      { id: 'bass-deep', name: 'Бас: глубокий', nameEn: 'Bass: deep', phrasing: 'deep round bass, subdued' },
    ],
  },
  {
    id: 'guitars', name: 'Гитары', nameEn: 'Guitars',
    items: [
      { id: 'gtr-fuzz', name: 'Гитара: фузз-стена', nameEn: 'Guitar: fuzz wall', phrasing: 'fuzz wall of guitars' },
      { id: 'gtr-clean', name: 'Гитара: чистый арпеджио', nameEn: 'Guitar: clean arpeggio', phrasing: 'clean arpeggiated guitar' },
      { id: 'gtr-trem', name: 'Гитара: тремоло', nameEn: 'Guitar: tremolo', phrasing: 'tremolo-picked guitar' },
      { id: 'gtr-acoustic', name: 'Акустика', nameEn: 'Acoustic', phrasing: 'strummed acoustic guitar' },
      { id: 'gtr-slide', name: 'Слайд', nameEn: 'Slide', phrasing: 'slide guitar lines' },
    ],
  },
  {
    id: 'keys', name: 'Клавиши', nameEn: 'Keys',
    items: [
      { id: 'organ-hammond', name: 'Хэммонд', nameEn: 'Hammond', phrasing: 'Hammond organ swells' },
      { id: 'synth-pad', name: 'Синт-пэд', nameEn: 'Synth pad', phrasing: 'warm analog synth pad' },
      { id: 'piano-upright', name: 'Пианино (прямое)', nameEn: 'Upright piano', phrasing: 'detuned upright piano' },
      { id: 'rhodes', name: 'Родес', nameEn: 'Rhodes', phrasing: 'Rhodes electric piano' },
    ],
  },
  {
    id: 'folk', name: 'Народные', nameEn: 'Folk',
    items: [
      { id: 'flute', name: 'Флейта', nameEn: 'Flute', phrasing: 'airy flute melody' },
      { id: 'accordion', name: 'Акордеон', nameEn: 'Accordion', phrasing: 'wistful accordion' },
      { id: 'violin', name: 'Скрипка', nameEn: 'Violin', phrasing: 'folk violin, rough bowing' },
      { id: 'cello', name: 'Виолончель', nameEn: 'Cello', phrasing: 'dark cello drones' },
    ],
  },
  {
    id: 'electro', name: 'Электроника', nameEn: 'Electronic',
    items: [
      { id: 'synth-bass', name: 'Синт-бас', nameEn: 'Synth bass', phrasing: 'moogy synth bass' },
      { id: 'arp', name: 'Арпеджиатор', nameEn: 'Arpeggiator', phrasing: 'sequenced synth arpeggio' },
      { id: 'noise', name: 'Шумовой слой', nameEn: 'Noise layer', phrasing: 'tape hiss and noise layer' },
    ],
  },
  {
    id: 'special', name: 'Спецслои', nameEn: 'Special layers',
    items: [
      { id: 'choir', name: 'Бэк-вокал/хор', nameEn: 'Backing vocals/choir', phrasing: 'distant choir backing vocals' },
      { id: 'glockenspiel', name: 'Колокольчики', nameEn: 'Glockenspiel', phrasing: 'sparse glockenspiel' },
      { id: 'field', name: 'Полевые записи', nameEn: 'Field recordings', phrasing: 'field recording interjections' },
    ],
  },
]

export const rackEffects = [
  { id: '', name: 'чисто', nameEn: 'clean' },
  { id: 'reverb', name: 'реверб', nameEn: 'reverb' },
  { id: 'fuzz', name: 'фузз', nameEn: 'fuzz' },
  { id: 'tremolo', name: 'тремоло', nameEn: 'tremolo' },
  { id: 'phaser', name: 'фэйзер', nameEn: 'phaser' },
  { id: 'wah', name: 'вау', nameEn: 'wah' },
  { id: 'echo', name: 'эхо', nameEn: 'echo' },
]

const effectPhrase = {
  reverb: 'drenched in reverb',
  fuzz: 'through fuzz',
  tremolo: 'with tremolo',
  phaser: 'with slow phaser',
  wah: 'with wah',
  echo: 'with tape echo',
}

export function allRackItems() {
  return rackGroups.flatMap(g => g.items)
}

// Стойка → фрагмент строки стиля
export function rackCompile(sel) {
  const items = allRackItems()
  return sel.map(({ id, effect }) => {
    const it = items.find(x => x.id === id)
    if (!it) return ''
    return it.phrasing + (effect && effectPhrase[effect] ? ` ${effectPhrase[effect]}` : '')
  }).filter(Boolean).join(', ')
}
