// ТК7: renderJob(job, jobs) — какие поля трека идут в рендер студии.
// Версия-вариант (role 'variant': DSP/пресет) своих стиля/текста/сида не имеет — их берём у ближайшего
// предка по parent_id с role ≠ 'variant'; id, название, длительность, план, parent_id и role — от самой версии.
// Дефект: #749/#750 из варианта #467 ушли со стилем «(вариант DSP-эффекта)» и без текста.
import { describe, test, expect } from 'vitest'
import { renderJob } from './requests.js'

// строка джобы как в GET /jobs воркера
function track(over = {}) {
  return {
    id: 1, role: '', parent_id: null, title: 'Warm Wire',
    style: 'post-punk, baritone, male vocal', lyrics: '[verse]\nwire is warm\n',
    seed: 42, cot: true, duration_sec: 180, abc_file: 'plan-1.abc',
    ...over,
  }
}

// вариант DSP-эффекта: стиля/текста/сида нет
function variant(over = {}) {
  return track({
    role: 'variant', style: '(вариант DSP-эффекта)', lyrics: '', seed: null, cot: false,
    ...over,
  })
}

const snap = (v) => JSON.parse(JSON.stringify(v))

describe('renderJob — не вариант', () => {
  test.each(['', 'rebuild', 'continue'])('role %j → тот же объект без изменений', (role) => {
    const job = track({ id: 5, role, parent_id: role ? 1 : null })
    const jobs = [track({ id: 1, style: 'другой стиль' }), job]
    const before = snap(job)
    const out = renderJob(job, jobs)
    expect(out).toBe(job)
    expect(out).toEqual(before)
  })
})

describe('renderJob — вариант берёт стиль/текст/сид/cot у исходного трека', () => {
  test('вариант прямо от трека', () => {
    const src = track({ id: 467, style: 'shoegaze', lyrics: 'la la', seed: 7, cot: true })
    const v = variant({ id: 749, parent_id: 467, title: 'Warm Wire (FX)', duration_sec: 95, abc_file: 'v.abc' })
    const out = renderJob(v, [src, v])
    expect(out).toMatchObject({ style: 'shoegaze', lyrics: 'la la', seed: 7, cot: true })
  })

  test('цепочка из трёх вариантов → поля трека-основы, а не промежуточных версий', () => {
    const src = track({ id: 10, style: 'pub rock', lyrics: 'hammer it down', seed: 1234, cot: false })
    const v1 = variant({ id: 11, parent_id: 10, style: '(вариант DSP-эффекта)' })
    const v2 = variant({ id: 12, parent_id: 11 })
    const v3 = variant({ id: 13, parent_id: 12, title: 'Третья версия' })
    // порядок в списке — не по цепочке
    const out = renderJob(v3, [v2, src, v3, v1])
    expect(out).toMatchObject({ style: 'pub rock', lyrics: 'hammer it down', seed: 1234, cot: false })
  })

  test('цепочка из двух вариантов', () => {
    const src = track({ id: 20, style: 'synthwave', lyrics: 'neon', seed: 0, cot: true })
    const v1 = variant({ id: 21, parent_id: 20 })
    const v2 = variant({ id: 22, parent_id: 21 })
    const out = renderJob(v2, [src, v1, v2])
    // seed 0 — настоящий сид, не «пусто»
    expect(out).toMatchObject({ style: 'synthwave', lyrics: 'neon', seed: 0, cot: true })
  })

  test('предок — пересборка (rebuild): поля берутся у неё, а не у её родителя', () => {
    const orig = track({ id: 30, style: 'old style', lyrics: 'old words', seed: 1, cot: false })
    const rb = track({ id: 31, role: 'rebuild', parent_id: 30, style: 'new style', lyrics: 'new words', seed: 2, cot: true })
    const v = variant({ id: 32, parent_id: 31 })
    const out = renderJob(v, [orig, rb, v])
    expect(out).toMatchObject({ style: 'new style', lyrics: 'new words', seed: 2, cot: true })
  })

  test('id, название, длительность, план, parent_id и role — от самой версии', () => {
    const src = track({ id: 40, title: 'Исходник', duration_sec: 200, abc_file: 'src.abc' })
    const v1 = variant({ id: 41, parent_id: 40, title: 'Промежуточная', duration_sec: 150, abc_file: 'mid.abc' })
    const v2 = variant({ id: 42, parent_id: 41, title: 'Моя версия', duration_sec: 95.5, abc_file: 'mine.abc' })
    const out = renderJob(v2, [src, v1, v2])
    expect(out).toMatchObject({
      id: 42, title: 'Моя версия', duration_sec: 95.5, abc_file: 'mine.abc', parent_id: 41, role: 'variant',
    })
  })

  test('лишние поля версии сохраняются', () => {
    const src = track({ id: 50 })
    const v = variant({ id: 51, parent_id: 50, status: 'done', audio: 'a.wav' })
    const out = renderJob(v, [src, v])
    expect(out).toMatchObject({ status: 'done', audio: 'a.wav' })
  })

  test('вход не мутируется: ни версия, ни список, ни предки', () => {
    const src = track({ id: 60, style: 'x', lyrics: 'y', seed: 3 })
    const v1 = variant({ id: 61, parent_id: 60 })
    const v2 = variant({ id: 62, parent_id: 61 })
    const jobs = [src, v1, v2]
    const before = snap(jobs)
    const out = renderJob(v2, jobs)
    expect(out).not.toBe(v2)
    expect(snap(jobs)).toEqual(before)
    expect(jobs).toHaveLength(3)
    expect(jobs[2]).toBe(v2)
  })
})

describe('renderJob — исходник не найти: версия как есть', () => {
  function expectUnchanged(job, jobs) {
    const before = snap(job)
    const jobsBefore = snap(jobs)
    const out = renderJob(job, jobs)
    expect(out).toEqual(before)
    expect(snap(jobs)).toEqual(jobsBefore)
  }

  test('предка нет в списке', () => {
    const v = variant({ id: 70, parent_id: 999 })
    expectUnchanged(v, [track({ id: 1 }), v])
  })

  test('цепочка вариантов обрывается на отсутствующем предке', () => {
    const v1 = variant({ id: 71, parent_id: 998 })
    const v2 = variant({ id: 72, parent_id: 71 })
    expectUnchanged(v2, [v1, v2])
  })

  test('цикл parent_id между вариантами — без зависания', () => {
    const a = variant({ id: 80, parent_id: 81 })
    const b = variant({ id: 81, parent_id: 80 })
    expectUnchanged(a, [a, b])
  })

  test('вариант ссылается сам на себя', () => {
    const a = variant({ id: 82, parent_id: 82 })
    expectUnchanged(a, [a])
  })

  test('parent_id null', () => {
    const v = variant({ id: 90, parent_id: null })
    expectUnchanged(v, [track({ id: 1 }), v])
  })

  test('parent_id отсутствует', () => {
    const v = variant({ id: 91 })
    delete v.parent_id
    expectUnchanged(v, [track({ id: 1 }), v])
  })

  test('пустой список джоб', () => {
    const v = variant({ id: 92, parent_id: 1 })
    expectUnchanged(v, [])
  })
})
