// «Переиграть в другом стиле» (бывший овердаб студии): YuE заново играет трек по его нотам в новом стиле — кавер
// новым треком или слой поверх оригинала. Чистая логика без DOM: форма → запрос SubmitOverdub.

export const REPLAY_INSTRUMENTAL = '[Instrumental]'
const GAIN_MIN = 0.1
const GAIN_MAX = 1

/** Форма {genre, chips: [en], text, voice: 'lyrics'|'none'|'own', ownLyrics, mix: 'cover'|'layer', gain, close} →
 *  {style, lyrics, gain, seed}; стиля нет — null. Кавер — gain 0 (воркер не смешивает с оригиналом); «ближе к
 *  оригиналу» — сид трека, «новая интерпретация» — 0 (воркер возьмёт случайный). */
export function replayRequest(form, job) {
  const f = form || {}
  const j = job || {}
  const style = [f.genre, ...(f.chips || []), (f.text || '').trim()].map((s) => String(s || '').trim())
    .filter(Boolean).join(', ')
  if (!style) return null
  let lyrics = j.lyrics || ''
  if (f.voice === 'none') lyrics = REPLAY_INSTRUMENTAL
  else if (f.voice === 'own') lyrics = (f.ownLyrics || '').trim() || j.lyrics || ''
  const gain = f.mix === 'layer' ? Math.min(GAIN_MAX, Math.max(GAIN_MIN, Number(f.gain) || 0.5)) : 0
  return { style, lyrics, gain, seed: f.close ? (j.seed || 0) : 0 }
}
