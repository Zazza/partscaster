// «Что нового» — коротко и простыми словами, новые версии сверху. Запись
// добавляется при релизе вместе с разделом CHANGELOG (там — подробно для
// разработчиков). Первая запись = версия этой сборки (см. welcome.js).
export const RELEASE_NOTES = [
  {
    version: '0.8.0',
    ru: [
      'Теперь мы Partscaster: новое имя — та же программа. Настройки переносятся сами.',
      'Список жанров больше не обрезается: виден весь алфавит, металл нашёлся. Добавлены блэк-метал, дэт-дум, DSBM, блэкгейз, металкор и ещё десять.',
      'Библиотека стилей — по алфавиту и на языке интерфейса; стойка тоже переведена.',
      'Кнопка «Создать партитуру» у треков без неё — после неё работает пиано-ролл.',
      'Свой значок приложения и единые значки вместо эмодзи.',
      'Строка состояния внизу: модель, видеопамять, очередь.',
      'Список треков — строками; раскрытый трек разбит на «Действия · Стиль · Версии».',
      'Педали для гитары и 23 новых эффекта против «плоского» звука.',
      'Гитара и клавиши — отдельными дорожками.',
      '«Характер» исполнения: смелее или точнее.',
      'Студия по шагам: слушать · править · звук · готово.',
      'Готовую ABC-партитуру можно прикрепить прямо в форме нового трека.',
      'Распознавание нот, дорожки и текст больше не мешают рендеру — общая очередь к видеокарте.',
    ],
    en: [
      'We are Partscaster now: a new name, the same app. Your settings carry over.',
      'The genre list is no longer cut off: the whole alphabet is visible, metal included. Added black metal, death doom, DSBM, blackgaze, metalcore and ten more.',
      'The style library is alphabetical and follows the interface language; the rack is translated too.',
      '“Create score” for tracks without one — the piano roll works after it.',
      'An app icon of its own and one icon set instead of emoji.',
      'Status bar at the bottom: model, video memory, queue.',
      'Track list as rows; an opened track is split into “Actions · Style · Versions”.',
      'Guitar pedals and 23 new effects against a “flat” sound.',
      'Guitar and keys as separate stems.',
      'Performance “character”: bolder or more precise.',
      'Studio in steps: listen · edit · sound · done.',
      'A ready ABC score can be attached right in the new-track form.',
      'Note recognition, stems and lyrics no longer get in the way of rendering — one shared GPU queue.',
    ],
  },
]
