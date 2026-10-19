// Модель генерации (yue2.cpp): метка модели у трека, предупреждение «повторить» на другой модели,
// список моделей для настроек. Чистые функции; тексты — через общий i18n.
import { useI18n } from './i18n/index.js'

const MODEL_ORDER = ['bf16', 'q8', 'q6', 'q5']
// старые треки (engine "" или "torch") сделаны прежним движком на PyTorch
const LEGACY = 'PyTorch'

/** Модель трека: "cpp-q8" → "q8"; старые треки — ''. */
function trackModel(job) {
  const e = (job && job.engine) || ''
  return e.startsWith('cpp-') ? e.slice(4) : ''
}

/** Текущая модель воркера из его конфига; конфиг не загружен — ''. */
export function currentModel(config) {
  return (config && config.cpp_model) || ''
}

/**
 * Метка модели у трека — только когда она отличается от текущей (повтор по сиду прозвучит иначе):
 * «BF16», «Q8»…; старые треки — «PyTorch». Конфиг не загружен — '' (ложной метки нет).
 */
export function modelBadge(job, config) {
  const cur = currentModel(config)
  if (!cur) return ''
  const m = trackModel(job)
  if (m === cur) return ''
  return m ? m.toUpperCase() : LEGACY
}

/** Предупреждение перед «Повторить»: сид повторяет исполнение только на той же модели. */
export function retryWarning(job, config) {
  return modelBadge(job, config) ? useI18n().t('engine.retryWarning') : ''
}

/**
 * Модели для выбора: [{value, label, installed, recommended}] в порядке bf16, q8, q6, q5; нет cpp_models — [].
 * recommended — рекомендуемая для карты воркера (cpp_recommended); выбрать можно любую.
 */
export function modelOptions(config) {
  const models = (config && config.cpp_models) || {}
  const rec = (config && config.cpp_recommended && config.cpp_recommended.model) || ''
  const { t, locale } = useI18n()
  return MODEL_ORDER.filter((n) => models[n]).map((n) => {
    const m = models[n]
    const gb = (Number(m.size || 0) / 1e9).toFixed(1)
    const size = locale.value === 'ru' ? gb.replace('.', ',') : gb
    const state = m.installed ? t('engine.model.installed') : t('engine.model.notInstalled')
    return {
      value: n,
      installed: !!m.installed,
      recommended: n === rec,
      label: t('engine.modelOption', {
        name: n.toUpperCase(),
        size,
        state: n === rec ? state + ', ' + t('engine.model.recommended') : state,
      }),
    }
  })
}

/** Что отправить воркеру при сохранении: модель и экономный режим. */
export function modelConfig(model, lowVram) {
  return { cpp_model: model, cpp_low_vram: !!lowVram }
}
