// Рецепт трека: превью файла перед импортом и строки хода импорта для списка треков.

// recipePreview — что покажет окно импорта: recipe — объект или текст файла, workerModel — модель воркера
// этой установки (из /version; '' — неизвестна). Не рецепт — { error }.
export function recipePreview(recipe, workerModel) {
  let r = recipe
  if (typeof r === 'string') {
    try { r = JSON.parse(r) } catch { return { error: 'файл не читается как рецепт трека' } }
  }
  if (!r || r.format !== 'yue-track-recipe') return { error: 'это не рецепт трека Partscaster' }
  if (r.version !== 1) return { error: `версия рецепта ${r.version} не поддерживается — обновите приложение` }
  const title = r.track?.title || r.root?.title || 'трек'
  const steps = 1 + (Array.isArray(r.ops) ? r.ops.length : 0)
  const warnings = Array.isArray(r.warnings) ? r.warnings : []
  // модель другая — исполнение по сиду может отличаться заметнее: предложить «по плану»
  const modelDiffers = !!(workerModel && r.worker_model && workerModel !== r.worker_model)
  return { title, steps, warnings, modelDiffers, model: r.worker_model || '' }
}

// activeImports — незаконченные и недавно упавшие импорты для строки над списком треков
export function activeImports(list) {
  return (list || []).filter((x) => x.status === 'running' || x.status === 'error')
}
