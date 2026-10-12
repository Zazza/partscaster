// Второй движок генерации yue2.cpp (ЭКСПЕРИМЕНТ): метка трека, предупреждение «повторить» на другом движке,
// список моделей для настроек. Чистые функции; тексты — через общий i18n.
import { useI18n } from './i18n/index.js'

const MODEL_ORDER = ['bf16', 'q8', 'q6', 'q5']

/** Метка движка трека: «yue2.cpp Q8»; PyTorch и старые треки (engine "") — ''. */
export function engineLabel(job) {
  const e = (job && job.engine) || ''
  return e.startsWith('cpp-') ? 'yue2.cpp ' + e.slice(4).toUpperCase() : ''
}

/** Движок трека в виде метки: у старых треков пусто — это PyTorch. */
function trackEngine(job) {
  return (job && job.engine) || 'torch'
}

/**
 * Предупреждение перед «Повторить»: сид повторяет исполнение только на том же движке и той же модели.
 * currentEngine — действующий движок ("torch" | "cpp-q8" …); неизвестен (конфиг не загружен) — ''.
 */
export function retryWarning(job, currentEngine) {
  if (!currentEngine || trackEngine(job) === currentEngine) return ''
  return useI18n().t('engine.retryWarning')
}

/** Действующий движок из конфига воркера в виде метки трека ("torch" | "cpp-q8"); старый воркер — ''. */
export function currentEngineLabel(config) {
  if (!config || !config.engine) return ''
  return config.engine === 'cpp' ? 'cpp-' + (config.cpp_model || 'q8') : 'torch'
}

/** Модели yue2.cpp для выбора: [{value, label, installed}] в порядке bf16, q8, q6, q5; нет cpp_models — []. */
export function modelOptions(config) {
  const models = (config && config.cpp_models) || {}
  const { t, locale } = useI18n()
  return MODEL_ORDER.filter((n) => models[n]).map((n) => {
    const m = models[n]
    const gb = (Number(m.size || 0) / 1e9).toFixed(1)
    const size = locale.value === 'ru' ? gb.replace('.', ',') : gb
    return {
      value: n,
      installed: !!m.installed,
      label: t('engine.modelOption', {
        name: n.toUpperCase(),
        size,
        state: m.installed ? t('engine.model.installed') : t('engine.model.notInstalled'),
      }),
    }
  })
}

/**
 * Что отправить воркеру при сохранении выбора движка: основной — только engine (модель yue2.cpp не трогается,
 * её скачанность не проверяется — иначе «Сохранить» без моделей падал бы 422); yue2.cpp — с моделью и режимом.
 */
export function engineConfig(engine, cppModel, cppLowVram) {
  if (engine !== 'cpp') return { engine: 'torch' }
  return { engine: 'cpp', cpp_model: cppModel, cpp_low_vram: !!cppLowVram }
}
