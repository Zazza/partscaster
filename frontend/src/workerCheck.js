// Строка результата кнопки «Проверить» в настройках: что ответил воркер по введённому адресу.
// res — CheckResult из Go (URL, Health, Version, Warning), err — текст ошибки ('' / null — нет).
export function workerCheckText(res, err) {
  if (err) {
    const where = res?.URL ? ` по адресу ${res.URL}` : ''
    return { ok: false, text: `воркер не отвечает${where} — запущен ли он и верен ли адрес?` }
  }
  const v = res?.Version
  const head = v ? `воркер ${v.version} (API ${v.api})` : 'воркер отвечает'
  // модель грузится в память при первой генерации, а не при старте воркера
  const model = res?.Health?.model_loaded ? 'модель в памяти' : 'модель не загружена — загрузится при первой генерации'
  const text = `${head} · ${model}` + (res?.Warning ? ` — ${res.Warning}` : '')
  return { ok: true, text }
}
