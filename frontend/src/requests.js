// Сборка заявок к воркеру — чистые функции «вход → объект заявки api» (без DOM и сети).
// Новый трек — форма App.vue (POST /jobs); студия — инструментал, «заново с места», «перепеть с
// места», эффект на дорожку (StudioPage.vue).
import { SLOT_ORDER, dictStyle, effectiveLyrics } from './styleLogic.js'
import { durTokens } from './slotOptions.js'
import { characterPayload } from './character.js'
import { beatSecAt, continuationPlan, planTimeline, trickStyleSuffix } from './abcEdit.js'
import { revoiceSpecKinds } from './vocalParts.js'

const CYR = /[а-яё]/i

// строка стиля формы до перевода: «стиль одной строкой» перекрывает всё; иначе слоты по порядку
// (русские значения из списков — словарём по-английски), стойка, BPM
export function formStyle(override, slots, rack) {
  if (override.trim()) return override.trim()
  const parts = SLOT_ORDER.map(k => (slots[k] || '').trim())
    .filter(Boolean).map(dictStyle)
  if (rack) parts.push(rack)
  if (slots.bpm) parts.push(slots.bpm + ' BPM')
  return parts.join(', ')
}

// самописная кириллица уходит в перевод (галочка «рус → eng»); английское — как есть
export const needsTranslate = (s, enabled) => !!enabled && CYR.test(s)

// перевод с откатом: сбой Ollama — стиль как есть, трек всё равно уходит
export async function translateOr(s, translate) {
  try {
    const r = await translate(s)
    return r.text
  } catch (e) {
    console.warn('translate failed, using as-is:', e)
    return s
  }
}

// заявка нового трека без стиля (стиль — formStyle/перевод); extra — {draft: true} у черновика
export function newTrackPayload(f, extra = {}) {
  const abc = f.abc.trim()
  return {
    title: f.title,
    lyrics: effectiveLyrics(f.lyrics, f.noLyrics, f.durMode),
    seed: f.seed ? Number(f.seed) : 0,
    // драматургии и прикреплённому ABC нужен план: off его не строит — молча повышаем до melody
    cot: (f.arc || abc) && f.cot === 'off' ? 'melody' : f.cot,
    arc: f.arc,
    // потолок длины из селектора длительности (0 = бюджет воркера)
    max_tokens: durTokens[f.durMode] || 0,
    ...characterPayload(f.temperature, f.cfg),
    ...(abc ? { abc } : {}),
    // пресеты звука — только у полного трека (черновик их не получает)
    ...(f.soundPresetIds.length && !extra.draft ? { sound_preset_ids: [...f.soundPresetIds] } : {}),
    ...extra,
  }
}

// стиль трека с припиской приёмов, которые план держит слабо (голос ±октава)
export function trickStyle(base, specs) {
  const suffix = trickStyleSuffix(specs)
  if (!suffix) return base
  const head = (base || '').trim().replace(/,$/, '')
  return head ? head + ', ' + suffix : suffix
}

// убрать голоса из текста ABC: строки V: <имя> и тела этих голосов выкидываются
export function dropVoices(abc, voiceNames) {
  const removed = new Set(voiceNames)
  const out = []
  let curVoice = null
  for (const line of abc.split('\n')) {
    const t = line.trim()
    const vm = t.match(/^V:\s*(\S+)/)
    if (vm) {
      curVoice = vm[1]
      if (removed.has(curVoice)) continue
      out.push(line)
      continue
    }
    if (!t || t.startsWith('%') || /^[A-Za-z]:/.test(t)) { out.push(line); continue }   // заголовок/метка секции/пустая
    if (curVoice && removed.has(curVoice)) continue                // тело убранного голоса
    out.push(line)
  }
  return out.join('\n')
}

// вокальные голоса плана (по порядку голосов ролла)
export const vocalVoices = (voiceOrder) => (voiceOrder || []).filter((v) => /vocal/i.test(v))

// инструментал: та же музыка (план, в т.ч. с накопленными приёмами), но без вокальной партии;
// стиль получает подсказку, лирика — [Instrumental]
export function instrumentalRequest(job, plan, vocals, specs) {
  return {
    title: (job.title || 'трек') + ' · инструментал',
    style: trickStyle(job.style, specs) + ', instrumental, no vocals',
    lyrics: '[Instrumental]',
    seed: job.seed || 0,   // 0 — воркер возьмёт сид родителя
    cot: job.cot === 'off' ? 'melody' : job.cot,
    abc: dropVoices(plan, vocals),
    parent_id: job.id, role: 'rebuild',
  }
}

// «заново с места» (POST /jobs/{id}/continue): с начала выделения, другой сид (0 — выберет воркер);
// приёмы ролла — изменённым планом (ровно по разу поверх исходного), приписка к стилю — без пробелов по краям
export function continueRequest(job, sel, baseAbc, specs, styleAdd) {
  return {
    jobId: job.id,
    from: sel.from,
    seed: 0,
    abc: specs.length ? continuationPlan(baseAbc, specs) : '',
    styleAdd: styleAdd.trim(),
  }
}

// «сыграть кусок с правками» [from, to) с, в контексте трека: звук до начала куска — как уже сыгранный, дальше модель
// играет по полному плану с правками только до конца куска. Кусок раньше FRAGMENT_MIN_FROM_S — продолжать не с чего:
// обычный рендер (POST /jobs, role 'fragment') по полному плану, бюджет шагов — до конца куска плюс хвост, как у
// воркера; иначе — POST /jobs/{id}/fragment. Приёмы, которые план держит слабо, — припиской к стилю.
export const FRAGMENT_MIN_FROM_S = 1     // кусок раньше — с начала трека
export const FRAGMENT_TAIL_S = 2         // как у воркера: после конца куска ещё 2 с
export const SEM_TOK_PER_SEC = 25        // шагов модели в секунду звука (воркер SEM_TOK_PER_SEC)

/** Окно куска: выделение на волне, иначе последний накопленный приём; нечего играть — null. Импорт (без шагов
 *  модели) ни продолжить, ни сыграть по стилю-заглушке нельзя — { noTokens: true }, до любой заявки. */
export function fragmentWindow(sel, specs, isImport) {
  const w = sel || (specs.length ? specs[specs.length - 1] : null)
  if (!w) return null
  if (isImport) return { noTokens: true }
  return { from: w.from, to: w.to }
}

/** { submit } — заявка POST /jobs, либо { fragment: {jobId, from, to, seed, abc, styleAdd} } — /fragment. */
export function fragmentRequest(job, from, to, baseAbc, specs) {
  const abc = specs.length ? continuationPlan(baseAbc, specs) : ''
  if (from < FRAGMENT_MIN_FROM_S) {
    return {
      submit: {
        title: (job.title || 'трек') + ' · кусок', style: trickStyle(job.style, specs), lyrics: job.lyrics,
        seed: job.seed || 0, cot: job.cot === 'off' ? 'melody' : job.cot,
        abc: abc || baseAbc, max_tokens: Math.round((to + FRAGMENT_TAIL_S) * SEM_TOK_PER_SEC),
        parent_id: job.id, role: 'fragment',
      },
    }
  }
  return { fragment: { jobId: job.id, from, to, seed: job.seed || 0, abc, styleAdd: trickStyleSuffix(specs) } }
}

// число тактов вокальных голосов плана: у версии и источника голоса оно должно совпадать
export const vocalBars = (abc) =>
  Object.entries(planTimeline(abc)).filter(([v]) => /vocal/i.test(v)).map(([, l]) => l.length).join()

// «перепеть с места»: план источника голоса с приёмами голоса (прочие приёмы не применяются),
// доля для выравнивания по плану версии, подпись «что сделано» (нет приёмов — takeLabel)
export function revoicePlan(srcAbc, baseAbc, specs, sel, takeLabel) {
  const own = specs.filter((s) => revoiceSpecKinds.has(s.kind))
  return {
    abc: continuationPlan(srcAbc, own),
    beat: beatSecAt(baseAbc, sel.from),
    what: own.map((s) => s.label).join(', ') || takeLabel,
  }
}

// запись для сервиса useRevoice: голос дубля id подставляется в окно выделения версии job
export function revoiceRecord(job, id, sel, beat, what) {
  return { parent: job.id, child: id, from: sel.from, to: sel.to, beat, voiceSrc: id,
    title: `${job.title || 'трек'} · голос: ${what} (дубль #${id})` }
}

// эффект на дорожку (запись реестра пересборки): окно — выделение, без выделения — вся дорожка
export function stemFxRequest(stem, chainId, params, sel) {
  return { stem, chain: chainId, params: { ...(params || {}) }, from: sel ? sel.from : 0, to: sel ? sel.to : 0 }
}
