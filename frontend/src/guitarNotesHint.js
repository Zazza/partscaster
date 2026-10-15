// Гитара по нотам (блок gtrnotes): подсказка, какими моделями воркер распознаёт ноты гитары трека — или что
// распознавание не установлено (worker/notes_install.sh). Чистая логика без DOM.

/** chain — цепочка блоков; cfg — /config воркера (null — старый воркер или нет связи); t — перевод.
 *  Нет блока gtrnotes — ''. basic-pitch идёт в окружении распознавания всегда, YourMT3+ — если скачан. */
export function notesHint(chain, cfg, t) {
  if (!(chain || []).some((b) => b && b.type === 'gtrnotes')) return ''
  if (!cfg || cfg.notes_available !== true) return t('gnotes.missing')
  return (cfg.notes_engines || []).includes('yourmt3') ? t('gnotes.both') : t('gnotes.bp')
}
