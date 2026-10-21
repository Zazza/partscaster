// Окно установки воркера: шаги и ход установки. Вся логика установки — в Go (Installer, workerexe.RunInstall);
// здесь только показ: методы — window.go.main.Installer, ход — события stage/line/failed/done.
'use strict'

const api = () => window.go.main.Installer
const $ = (id) => document.getElementById(id)

// Тексты окна: язык — info.lang (программа берёт язык интерфейса Windows). {имя} — подстановка в L().
const I18N = {
  ru: {
    'sub': 'Установка воркера',
    'welcome.title': 'Добро пожаловать',
    'welcome.text': 'Воркер — часть Partscaster, которая генерирует музыку на видеокарте этого компьютера. Установка займёт несколько гигабайт и некоторое время — всё ставится в одну папку, в систему ничего не прописывается.',
    'gpu': 'Видеокарта: {gpu}, {vram} ГБ видеопамяти.',
    'home.title': 'Папка установки',
    'home.text': 'Сюда лягут Python, пакеты, модели и ваши треки.',
    'home.browse': 'Выбрать…',
    'home.need': 'Нужно около {need} ГБ; свободное место узнать не удалось.',
    'home.free': 'Свободно {free} ГБ, нужно около {need} ГБ.',
    'home.low': ' Места может не хватить.',
    'home.choose': 'Выберите папку установки',
    'model.title': 'Модель генерации',
    'model.file': ' — файл {size} ГБ',
    'model.rec': 'рекомендуется для этой видеокарты',
    'model.low': 'Экономный режим — память под карту (на 8 ГБ ~6,5 ГБ, трек с продолжениями до ~10 мин)',
    'model.hint': 'Модель можно сменить потом в настройках приложения.',
    'access.title': 'Доступ',
    'access.local': 'Только этот компьютер',
    'access.local.hint': 'приложение Partscaster стоит здесь же',
    'access.lan': 'С других компьютеров в сети',
    'access.lan.hint': 'приложение на другом компьютере; можно сменить в меню значка в трее',
    'install.title': 'Установка',
    'install.failed': 'Установка не закончилась',
    'install.retryHint': 'Уже скачанное не пропадёт — «Повторить» продолжит с того же места.',
    'install.more': 'Подробнее ▾',
    'install.less': 'Скрыть подробности ▴',
    'install.progress': '{file} — {done} из {total} ({pct}%)',
    'stage.0': 'Python и пакеты',
    'stage.1': 'Разделение на дорожки',
    'stage.2': 'Модель генерации',
    'stage.3': 'Запуск',
    'done.title': 'Готово',
    'done.text': 'Воркер запустится и будет работать в фоне — его значок появится в трее (у часов). Оттуда — журнал, доступ из сети и выход.',
    'done.addr': 'В приложении Partscaster укажите адрес воркера:',
    'done.copy': 'Скопировать',
    'done.copied': 'Скопировано',
    'btn.back': 'Назад',
    'btn.retry': 'Повторить',
    'btn.openlog': 'Открыть журнал',
    'btn.next': 'Далее',
    'btn.install': 'Установить',
    'btn.close': 'Закрыть',
    'unit.gb': 'ГБ',
    'unit.mb': 'МБ',
    'unit.kb': 'КБ',
  },
  en: {
    'sub': 'Worker setup',
    'welcome.title': 'Welcome',
    'welcome.text': 'The worker is the part of Partscaster that generates music on this computer’s graphics card. Setup takes several gigabytes and some time — everything goes into one folder, nothing is written into the system.',
    'gpu': 'Graphics card: {gpu}, {vram} GB of video memory.',
    'home.title': 'Install folder',
    'home.text': 'Python, packages, models and your tracks will go here.',
    'home.browse': 'Browse…',
    'home.need': 'About {need} GB is needed; free space could not be determined.',
    'home.free': '{free} GB free, about {need} GB needed.',
    'home.low': ' There may not be enough space.',
    'home.choose': 'Choose an install folder',
    'model.title': 'Generation model',
    'model.file': ' — {size} GB file',
    'model.rec': 'recommended for this graphics card',
    'model.low': 'Economy mode — memory sized to the card (~6.5 GB on 8 GB, a track with continuations up to ~10 min)',
    'model.hint': 'The model can be changed later in the app settings.',
    'access.title': 'Access',
    'access.local': 'This computer only',
    'access.local.hint': 'the Partscaster app is installed here too',
    'access.lan': 'From other computers on the network',
    'access.lan.hint': 'the app is on another computer; can be changed in the tray icon menu',
    'install.title': 'Installing',
    'install.failed': 'The installation did not finish',
    'install.retryHint': 'What is already downloaded is kept — “Retry” continues from the same point.',
    'install.more': 'Details ▾',
    'install.less': 'Hide details ▴',
    'install.progress': '{file} — {done} of {total} ({pct}%)',
    'stage.0': 'Python and packages',
    'stage.1': 'Stem separation',
    'stage.2': 'Generation model',
    'stage.3': 'Start',
    'done.title': 'Done',
    'done.text': 'The worker will start and run in the background — its icon appears in the tray (by the clock). From there: the log, network access and exit.',
    'done.addr': 'In the Partscaster app, enter the worker address:',
    'done.copy': 'Copy',
    'done.copied': 'Copied',
    'btn.back': 'Back',
    'btn.retry': 'Retry',
    'btn.openlog': 'Open log',
    'btn.next': 'Next',
    'btn.install': 'Install',
    'btn.close': 'Close',
    'unit.gb': 'GB',
    'unit.mb': 'MB',
    'unit.kb': 'KB',
  },
}
let lang = 'ru'
function L(key, vars) {
  let s = (I18N[lang] && I18N[lang][key]) ?? I18N.ru[key] ?? key
  for (const [k, v] of Object.entries(vars || {})) s = s.replaceAll('{' + k + '}', String(v))
  return s
}
function applyTexts() {
  document.documentElement.lang = lang
  for (const el of document.querySelectorAll('[data-i]')) el.textContent = L(el.dataset.i)
}
const STAGES = () => [0, 1, 2, 3].map((n) => L('stage.' + n))
const LOG_MAX = 3000 // строк журнала в окне (полный — в файле)

let info = null
let steps = []
let at = 0
let choice = null
let phase = 'choose' // choose | installing | failed | done

function show(name) {
  for (const s of document.querySelectorAll('.step')) s.hidden = s.id !== 's-' + name
}

function setError(text) {
  $('err').textContent = text || ''
  $('err').hidden = !text
}

function buttons() {
  const last = at === steps.length - 1
  $('back').hidden = phase !== 'choose' || at === 0
  $('retry').hidden = phase !== 'failed'
  $('openlog').hidden = phase !== 'failed'
  const next = $('next')
  next.hidden = phase === 'installing' || phase === 'failed'
  next.textContent = phase === 'done' || info.gpu_error ? L('btn.close') : last ? L('btn.install') : L('btn.next')
}

function gb(v) {
  const s = v.toFixed(1)
  return lang === 'ru' ? s.replace('.', ',') : s
}

async function refreshFree() {
  const free = await api().FreeGB($('home').value)
  const el = $('free')
  if (free < 0) {
    el.textContent = L('home.need', { need: info.need_gb })
    el.className = 'hint'
    return
  }
  el.textContent = L('home.free', { free: Math.floor(free), need: info.need_gb })
  el.className = free < info.need_gb ? 'hint warn' : 'hint'
  if (free < info.need_gb) el.textContent += L('home.low')
}

function renderModels() {
  const box = $('models')
  box.innerHTML = ''
  for (const m of info.models) {
    const label = document.createElement('label')
    label.className = 'opt'
    const size = m.size_gb ? L('model.file', { size: gb(m.size_gb) }) : ''
    label.innerHTML = `<input type="radio" name="model" value="${m.name}"> <b>${m.name.toUpperCase()}</b>` +
      `<span>${size}</span>` + (m.recommended ? ` <span class="rec">${L('model.rec')}</span>` : '')
    const input = label.querySelector('input')
    input.checked = m.name === info.model
    input.disabled = info.model_fixed
    box.appendChild(label)
  }
  $('low').checked = info.low_vram
  for (const r of document.querySelectorAll('input[name=lan]')) {
    r.checked = (r.value === '1') === info.lan
    r.disabled = info.lan_fixed
  }
}

function readChoice() {
  const m = document.querySelector('input[name=model]:checked')
  const lan = document.querySelector('input[name=lan]:checked')
  return {
    home: $('home').value.trim(),
    model: m ? m.value : info.model,
    low_vram: $('low').checked,
    lan: lan ? lan.value === '1' : info.lan,
  }
}

function renderStages() {
  $('stages').innerHTML = STAGES().map((s) => `<li>${s}</li>`).join('')
}

function markStage(n, cls) {
  const items = $('stages').children
  for (let i = 0; i < items.length; i++) {
    items[i].className = i < n ? 'ok' : i === n ? cls : ''
  }
}

let curStage = 0
function logLine(text) {
  const pre = $('log')
  pre.textContent += text + '\n'
  const lines = pre.textContent.split('\n')
  if (lines.length > LOG_MAX) pre.textContent = lines.slice(-LOG_MAX).join('\n')
  if (!pre.hidden) pre.scrollTop = pre.scrollHeight
  if (text.trim()) $('now').textContent = text
}

// человекочитаемые байты: «3,8 ГБ», «530 МБ» — сырые числа в окне не читаются
function human(n) {
  if (n >= 1024 ** 3) return gb(n / 1024 ** 3) + ' ' + L('unit.gb')
  if (n >= 1024 ** 2) return Math.round(n / 1024 ** 2) + ' ' + L('unit.mb')
  return Math.round(n / 1024) + ' ' + L('unit.kb')
}

function hideProgress() {
  $('prog').hidden = true
  $('prog-fill').style.width = '0'
}

window.runtime.EventsOn('progress', (p) => {
  const pct = p.total > 0 ? Math.floor((100 * p.done) / p.total) : 0
  $('prog').hidden = false
  $('prog-fill').style.width = pct + '%'
  $('prog-text').textContent = L('install.progress', { file: p.file, done: human(p.done), total: human(p.total), pct })
})

async function start() {
  setError('')
  // экран установки — до вызова: события этапов и быстрый сбой могут прийти раньше ответа Start
  const prev = phase
  phase = 'installing'
  $('fail').hidden = true
  $('inst-title').textContent = L('install.title')
  renderStages()
  curStage = 0
  markStage(0, 'cur')
  show('install')
  buttons()
  const err = await api().Start(choice)
  if (err) {
    // установка не началась (папка занята, нет места…): вернуться к выбору с текстом
    phase = prev === 'failed' ? 'failed' : 'choose'
    if (phase === 'choose' && steps.length) show(steps[at])
    setError(err)
    buttons()
  }
}

window.runtime.EventsOn('stage', (n) => {
  curStage = n
  markStage(n, 'cur')
  hideProgress()
  if (n < STAGES().length) $('now').textContent = STAGES()[n] + '…'
})
window.runtime.EventsOn('line', logLine)
window.runtime.EventsOn('failed', (text) => {
  phase = 'failed'
  markStage(curStage, 'bad')
  hideProgress()
  $('inst-title').textContent = L('install.failed')
  $('fail-text').textContent = text
  $('fail').hidden = false
  buttons()
})
window.runtime.EventsOn('done', (addr) => {
  phase = 'done'
  hideProgress()
  markStage(STAGES().length, '')
  $('addr').textContent = addr
  show('done')
  buttons()
})

$('next').addEventListener('click', async () => {
  if (phase === 'done' || info.gpu_error) {
    api().Close()
    return
  }
  if (steps[at] === 'home' && !$('home').value.trim()) {
    setError(L('home.choose'))
    return
  }
  setError('')
  if (at < steps.length - 1) {
    at++
    show(steps[at])
    buttons()
    return
  }
  choice = readChoice()
  await start()
})
$('back').addEventListener('click', () => {
  if (at > 0) at--
  setError('')
  show(steps[at])
  buttons()
})
$('retry').addEventListener('click', start)
$('openlog').addEventListener('click', () => api().OpenLog())
$('browse').addEventListener('click', async () => {
  $('home').value = await api().ChooseFolder($('home').value)
  refreshFree()
})
$('home').addEventListener('change', refreshFree)
$('copy').addEventListener('click', () => {
  api().Copy($('addr').textContent)
  $('copy').textContent = L('done.copied')
})
$('toggle-log').addEventListener('click', () => {
  const pre = $('log')
  pre.hidden = !pre.hidden
  $('toggle-log').textContent = pre.hidden ? L('install.more') : L('install.less')
  if (!pre.hidden) pre.scrollTop = pre.scrollHeight
})

async function init() {
  info = await api().Info()
  lang = info.lang === 'en' ? 'en' : 'ru'
  applyTexts()
  $('ver').textContent = info.version
  const gpu = $('gpu')
  if (info.gpu_error) {
    gpu.textContent = info.gpu_error
    gpu.className = 'card bad'
  } else {
    gpu.textContent = L('gpu', { gpu: info.gpu, vram: info.vram_gb })
  }
  $('home').value = info.home
  renderModels()
  steps = ['welcome']
  if (info.ask_home) steps.push('home')
  if (info.ask_choices) steps.push('choices')
  if (!info.ask_home && !info.ask_choices && !info.gpu_error) {
    // обновление: выбор уже сделан — сразу установка недостающего
    choice = readChoice()
    await start()
    return
  }
  at = 0
  show('welcome')
  buttons()
  if (info.ask_home) refreshFree()
}

init()
