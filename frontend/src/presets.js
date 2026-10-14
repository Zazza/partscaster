// Откалиброванные пресеты для групп библиотеки стилей.
// slots — декомпозиция для правки отдельных полей (скомпилируется заново);
// style — точная исходная строка калибровки (кнопка «строкой» — 1:1 воспроизведение).

export const presets = [
  {
    id: 'bigbeat-guitars',
    name: 'Электроника + жёсткие гитары', nameEn: 'Electronics + hard guitars',
    seed: 200204,
    style: 'Instrumental, no vocals, big beat electronic rock, breakbeat chopped funk drums, aggressive down-tuned distorted guitar riffs, moogy synth bass, pounding relentless, loudness war brickwall, intense aggression, rebellious defiance, 145 BPM',
    slots: {
      language: 'English', genre: 'big beat, industrial rock',
      rhythm: 'breakbeat, chopped funk drums, relentless',
      guitars: 'aggressive down-tuned distorted guitar riffs', keys: 'moogy synth bass',
      vocals: 'instrumental, no vocals',
      mood: 'intense aggression, swaggering confidence', production: 'loudness war brickwall, dense wall of sound',
    },
    bpm: 145,
    lyrics: '[Instrumental]',
  },
  {
    id: 'dark-techno',
    name: 'Тёмный техно', nameEn: 'Dark techno',
    seed: 200205,
    style: 'Instrumental, no vocals, dub techno dark warehouse, four-on-the-floor pounding kick, hypotonic dub chords with spacey delays, metallic clangorous hits, cavernous reverb, paranoid claustrophobia, 130 BPM',
    slots: {
      language: 'English', genre: 'dub techno, dark techno',
      rhythm: 'four-on-the-floor drum machine, relentless',
      guitars: '', keys: 'dub chords, spacey delays',
      vocals: 'instrumental, no vocals',
      mood: 'paranoid claustrophobia, hypnotic trance-like repetition', production: 'cavernous reverb, compressed radio sound',
    },
    bpm: 130,
    lyrics: '[Instrumental]',
  },
  {
    id: 'ethnic-oriental',
    name: 'Этника восточная', nameEn: 'Oriental ethnic',
    seed: 200207,
    style: 'Instrumental, no vocals, oriental world fusion, driving darbuka and frame drum groove, oud and bouzouki lines, duduk wails, warm live room, dark fairy tale atmosphere with sunny morning turn, 105 BPM',
    slots: {
      language: 'English', genre: 'world fusion, oriental',
      rhythm: 'ethnic percussion, driving darbuka groove',
      guitars: 'bouzouki lines', keys: 'oud, duduk',
      vocals: 'instrumental, no vocals',
      mood: 'dark fairy tale atmosphere, wistful wanderlust', production: 'warm live room recording',
    },
    bpm: 105,
    lyrics: '[Instrumental]',
  },
]
