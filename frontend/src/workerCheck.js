// Строка результата кнопки «Проверить» в настройках: что ответил воркер по введённому адресу.
// res — CheckResult из Go (URL, Health, Version, Warning), err — текст ошибки ('' / null — нет).
import { useI18n } from './i18n/index.js'

const { t } = useI18n()
export function workerCheckText(res, err) {
  if (err) {
    const text = res?.URL ? t('workerCheck.downAt', { url: res.URL }) : t('workerCheck.down')
    return { ok: false, text }
  }
  const v = res?.Version
  const head = v ? t('workerCheck.version', { v: v.version, api: v.api }) : t('workerCheck.ok')
  // модель грузится в память при первой генерации, а не при старте воркера
  const model = res?.Health?.model_loaded ? t('workerCheck.modelLoaded') : t('workerCheck.modelNotLoaded')
  const text = `${head} · ${model}` + (res?.Warning ? ` — ${res.Warning}` : '')
  return { ok: true, text }
}
