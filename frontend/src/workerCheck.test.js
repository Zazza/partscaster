// Тесты строки результата кнопки «Проверить» в настройках:
// воркер ответил — «воркер <версия> (API n) · модель в памяти / не загружена (загрузится при первой генерации)», с предупреждением
// о версии — оно в тексте; не ответил — понятная ошибка «воркер не отвечает» с советом
// проверить, запущен ли он и верен ли адрес (не сырой «connection refused»).
import { describe, it, expect } from 'vitest'
import { workerCheckText } from './workerCheck.js'

const URL = 'http://192.168.1.5:8091'

function res({ loaded = true, version = 'v0.12.0-3-gabc', api = 1, warning = '' } = {}) {
  return {
    URL,
    Health: { status: 'ok', model_loaded: loaded },
    Version: { version, api },
    Warning: warning,
  }
}

describe('workerCheckText: воркер ответил', () => {
  it('модель в памяти → ok, версия и API в тексте', () => {
    const r = workerCheckText(res(), null)
    expect(r.ok).toBe(true)
    expect(r.text).toContain('воркер v0.12.0-3-gabc')
    expect(r.text).toContain('API 1')
    expect(r.text).toContain('модель в памяти')
    expect(r.text).not.toContain('загружается')
  })

  // воркер грузит модель при первой генерации (yue_worker._get_pipe), не при старте — «загружается» было бы неправдой
  it('модель ещё не в памяти → «модель не загружена — загрузится при первой генерации»', () => {
    const r = workerCheckText(res({ loaded: false }), null)
    expect(r.ok).toBe(true)
    expect(r.text).toContain('загрузится при первой генерации')
    expect(r.text).not.toContain('модель в памяти')
  })

  it('номер API берётся из ответа, не константой', () => {
    expect(workerCheckText(res({ api: 3 }), null).text).toContain('API 3')
  })

  it('без предупреждения — текст без советов обновиться', () => {
    const t = workerCheckText(res(), null).text
    expect(t).not.toContain('обновите')
  })

  it('предупреждение о версии входит в текст (проверка всё равно успешна — плашка неблокирующая)', () => {
    const warning = 'воркер устарел — обновите воркер'
    const r = workerCheckText(res({ warning }), null)
    expect(r.ok).toBe(true)
    expect(r.text).toContain(warning)
    expect(r.text).toContain('воркер v0.12.0-3-gabc')
  })

  it('пустая строка ошибки не считается ошибкой', () => {
    expect(workerCheckText(res(), '').ok).toBe(true)
  })
})

describe('workerCheckText: воркер не ответил', () => {
  const ERR = 'Get "http://192.168.1.5:8091/health": dial tcp 192.168.1.5:8091: connect: connection refused'

  it('ошибка → ok:false и понятный текст с советом', () => {
    const r = workerCheckText(null, ERR)
    expect(r.ok).toBe(false)
    expect(r.text).toContain('воркер не отвечает')
    expect(r.text).toContain('запущен')
    expect(r.text).toContain('адрес')
  })

  it('адрес, по которому проверяли, — в тексте ошибки («по адресу … воркер не отвечает»)', () => {
    const r = workerCheckText({ URL }, ERR)
    expect(r.ok).toBe(false)
    expect(r.text).toContain(URL)
    expect(r.text).toContain('не отвечает')
  })

  it('при ошибке нет строки успеха', () => {
    const t = workerCheckText(null, ERR).text
    expect(t).not.toContain('модель в памяти')
    expect(t).not.toContain('модель не загружена')
  })
})
