// «4 Голос» — одна кнопка «прослушать кусок» на всё выбранное в блоке: что именно играть.
// Чистая логика без DOM: voice — выбран ли голос (карточка или «своё»), fx — выбрана ли обработка,
// probeReady/probeVoiceKey — готова ли проба и каким голосом спета, curVoiceKey — голос сейчас.
//   'wait'     — проба этим голосом уже рендерится: второй рендер не нужен, ждём её (сыграет по готовности);
//   'render'   — голос выбран, готовой пробы этим голосом нет: черновой рендер куска;
//   'probe'    — проба этого голоса готова: играет она (с эффектом, если выбран), рендера нет;
//   'trackFx'  — голоса нет, эффект есть: кусок трека с эффектом на голосе;
//   'original' — ничего не выбрано: оригинал трека с первого спетого такта.
export function voiceListenPlan({ voice, fx, probeReady, probePending, probeVoiceKey, curVoiceKey }) {
  if (voice) {
    const same = !!probeVoiceKey && probeVoiceKey === curVoiceKey
    if (same && probeReady) return 'probe'
    return same && probePending ? 'wait' : 'render'
  }
  return fx ? 'trackFx' : 'original'
}

// Запись того же эффекта на голос в черновике трека: шаг «Голос», дорожка vocals, та же цепочка, ручки (по
// значениям) и окно; нет — null. По самому списку, а не по памяти кнопки: убрали эффект из «Изменений трека» —
// его снова можно применить
export function voiceFxMatch(edits, fx, win) {
  if (!fx) return null
  const from = (win && win.from) || 0
  const to = (win && win.to) || 0
  // ручки — по значениям, порядок ключей не важен
  const same = (a, b) => {
    const x = a || {}
    const y = b || {}
    const kx = Object.keys(x)
    return kx.length === Object.keys(y).length && kx.every((k) => k in y && x[k] === y[k])
  }
  const all = (edits || []).filter((it) => it.step === 'voice' && it.chain === fx.chain && same(it.params, fx.params)
    && (it.stems || [])[0] === 'vocals' && (it.from || 0) === from && (it.to || 0) === to)
  // включённая запись важнее выключенной: эффект уже действует
  return all.find((it) => !it.off) || all[0] || null
}

// тот же эффект уже ДЕЙСТВУЕТ в черновике (выключенная запись — не в счёт: «Применить» её включит)
export function voiceFxApplied(edits, fx, win) {
  const m = voiceFxMatch(edits, fx, win)
  return !!m && !m.off
}
