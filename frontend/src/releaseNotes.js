// «Что нового» — коротко и простыми словами, новые версии сверху. Запись
// добавляется при релизе вместе с разделом CHANGELOG (там — подробно для
// разработчиков). Первая запись = версия этой сборки (см. welcome.js).
export const RELEASE_NOTES = [
  {
    version: '1.0.0',
    ru: [
      'Теперь мы Partscaster: новое имя — та же программа. Настройки переносятся сами.',
      'Генерация на yue2.cpp: хватает видеокарты от 8 ГБ, модель подбирается под карту при установке, экономный режим берёт столько памяти, сколько карта позволяет.',
      'Воркер на Windows без WSL: программа с окном установки и значком в трее.',
      '106 жанровых пресетов звука: одна кнопка меняет барабаны, гитару, педали и громкости по замерам настоящих записей. Пресеты идут в очередь — можно поставить несколько и сравнить.',
      'Гитарист по аккордам песни: бой, перебор, квинты, второй дубль. Барабанщик делает сбивки перед сменой частей.',
      'Верх выше 16 кГц, который модель срезает, достраивается — голос без «потолка»; тарелки в пресетах с живыми барабанами — настоящими сэмплами.',
      'Студия по шагам: слушать · править · звук · голос · готово. Любую правку можно прослушать до применения.',
      'Шаг «Голос» в студии: тот же трек другим голосом и эффекты на дорожку голоса.',
      'Гитарист и барабанщик работают и на своём импортированном треке.',
      'Гитарные педали, усилители и 50+ цепочек эффектов — на весь трек или одну дорожку.',
      'Новый вид: список треков таблицей, окно без системной рамки, очередь видеокарты в левом блоке.',
      'Окно «Что это» с примерами «было / стало» и горячие клавиши (пробел, ←/→).',
      'Английский интерфейс полностью переведён.',
      'Проще: убраны помощник на Ollama, профили исполнителей, «минус» и старый движок на PyTorch.',
    ],
    en: [
      'We are Partscaster now: a new name, the same app. Your settings carry over.',
      'Generation on yue2.cpp: a graphics card from 8 GB is enough, the model is picked for your card on install, and economy mode uses as much memory as the card allows.',
      'A Windows worker without WSL: a program with an install window and a tray icon.',
      '106 genre sound presets: one click changes the drums, guitar, pedals and levels measured on real records. Presets go through a queue — queue several and compare.',
      'A guitarist playing the song\'s chords: strumming, arpeggio, power chords, a second take. A drummer plays fills before section changes.',
      'The top above 16 kHz that the model cuts off is rebuilt — the voice loses its “ceiling”; presets with live drums play real cymbal samples.',
      'Studio in steps: listen · edit · sound · voice · done. Every edit can be heard before you apply it.',
      'The “Voice” step in the studio: the same track in another voice, and effects on the voice stem.',
      'The guitarist and the drummer work on your own imported track too.',
      'Guitar pedals, amps and 50+ effect chains — for the whole track or a single stem.',
      'A new look: the track list as a table, a window without the system frame, the GPU queue in the left panel.',
      'The “What it is” window with before / after examples, and hotkeys (space, ←/→).',
      'The English interface is fully translated.',
      'Simpler: the Ollama helper, artist profiles, “minus” and the old PyTorch engine are gone.',
    ],
  },
]
