// Тесты чистой логики редактора пресета звука: модель ⇄ тело сохранения ничего не теряет и не
// тащит служебных полей, каталог по дорожкам отсеивает партийные цепочки и мастер, дефолты стемов
// как во встроенных пресетах, источник предпрослушивания честен про части барабанов (RoFormer),
// на частях барабанов — только цепочка движка (иначе применение пресета откажется).
import { describe, expect, it } from 'vitest'
import { fxPresets } from './fxPresets.js'
import { PRESET_FAMILIES } from './soundPresets.js'
import {
  DRUM_PARTS, EDITOR_TARGETS, bodyFromModel, dupBody, editorCatalog, isPartItem,
  itemForTarget, masterPresets, newModel, presetMasterChain, previewSource, rhythmSpecs, specFromItem, specKindOk,
  specStemsFor, specKind, toModel, toggleStem, editorSnapshot,
} from './presetEdit.js'
import { fromWorkerChain, setParam } from './fxChain.js'
import BLOCKS from './fxBlocks.json'

// пресет со всеми видами записей и необязательными полями — roundtrip должен сохранить всё
const FULL = {
  id: 7, slug: 'x', builtin: true, family: 'Рок', name: 'Мой · тест', note: 'заметка',
  reference_job_id: 42, target_lufs: -14,
  specs: [
    { stems: ['bass'], engine: [{ type: 'comp', threshold_db: -20 }], db: 1.5, level_db: -6 },
    { stems: ['other'], chain: 'tape', params: { drive: 3 }, steps: [], db: 0 },
    { stems: ['vocals'], steps: [{ chain: 'softer', params: { mix: 0.4 }, off: true }, { chain: 'wider', params: {} }], db: -2 },
    { stems: ['guitar'], place: { pan: -0.3, width: 1.4 }, db: 0 },
  ],
  final: [{ chain: 'wall', params: { drive: 5 } }],
  master: [{ type: 'glue', ratio: 2 }, { type: 'limiter', target_lufs: -14, ceiling_db: -1 }],
  parts: [{ kind: 'synth', engine: [{ type: 'synth', wave: 'saw' }], style: 'pad', octave: 1 }],
}

describe('toModel/bodyFromModel — рецепт не теряется и не замусоривается', () => {
  it('roundtrip: тело сохраняет все записи с их видом, db, level_db, place, steps[].off и служебные поля', () => {
    const body = bodyFromModel(toModel(FULL))
    expect(body.name).toBe('Мой · тест')
    expect(body.note).toBe('заметка')
    expect(body.reference_job_id).toBe(42)
    expect(body.target_lufs).toBe(-14)
    expect(body.specs).toEqual(FULL.specs)
    expect(body.final).toEqual(FULL.final)
    expect(body.master).toEqual(FULL.master)
    expect(body.parts).toEqual(FULL.parts)
  })

  it('в теле нет id/slug/builtin/family и uid записей', () => {
    const body = bodyFromModel(toModel(FULL))
    for (const k of ['id', 'slug', 'builtin', 'family']) expect(body).not.toHaveProperty(k)
    for (const s of body.specs) expect(s).not.toHaveProperty('uid')
  })

  it('нейтральное место не пишется, пустой level_db не пишется; имя и заметка обрезаются по краям', () => {
    const body = bodyFromModel(toModel({ name: '  им  ', note: ' ', specs: [{ stems: ['bass'], db: 0 }] }))
    expect(body.name).toBe('им')
    expect(body.note).toBe('')
    expect(body.specs[0]).not.toHaveProperty('place')
    expect(body.specs[0]).not.toHaveProperty('level_db')
  })

  it('target_lufs нет — null (не undefined), пустой пресет валиден', () => {
    const body = bodyFromModel(newModel())
    expect(body.target_lufs).toBe(null)
    expect(body.specs).toEqual([])
    expect(body.parts).toEqual([])
  })

  it('исходный пресет модель не меняет (рабочие копии)', () => {
    const before = JSON.parse(JSON.stringify(FULL.specs))
    const m = toModel(FULL)
    m.specs[0].stems.push('drums')
    m.specs[0].engine.push({ type: 'gate' })
    expect(FULL.specs).toEqual(before)
  })
})

describe('dupBody — копия для «Сохранить как свой»', () => {
  it('имя с суффиксом не длиннее 80, рецепт целиком, без id/slug/builtin', () => {
    const body = dupBody({ ...FULL, name: 'Длинное-длинное-длинное-имя-пресета-которое-точно-превосходит-восемьдесят-символов-предела' }, ' (копия)')
    expect(body.name.length).toBeLessThanOrEqual(80)
    expect(body.name.endsWith(' (копия)')).toBe(true)
    expect(body.specs).toEqual(FULL.specs)
    expect(body.master).toEqual(FULL.master)
    expect(body.parts).toEqual(FULL.parts)
    expect(body.reference_job_id).toBe(42)
    for (const k of ['id', 'slug', 'builtin']) expect(body).not.toHaveProperty(k)
  })
})

describe('editorCatalog — каталог готовых цепочек по табу', () => {
  const groupsOf = (target) => editorCatalog(target).map((g) => g.group)
  const idsOf = (target) => editorCatalog(target).flatMap((g) => g.items.map((p) => p.id))

  it('у баса — группы bass-tone/bass-genre/bass-kit и оркестровый бас, партийных и мастеров нет', () => {
    expect(groupsOf('bass')).toEqual(expect.arrayContaining(['bass-tone', 'bass-genre', 'bass-kit', 'orch-bass']))
    expect(idsOf('bass')).toContain('bass-melodic')
    expect(idsOf('bass')).toContain('bass-kit')
    // всё, что попало в каталог, действительно про бас и не партийное
    for (const id of idsOf('bass')) {
      const p = fxPresets.find((x) => x.id === id)
      expect(p.stems.includes('bass'), id).toBe(true)
      expect(isPartItem(p), id).toBe(false)
    }
    expect(idsOf('bass')).not.toContain('master-glue')
  })

  it('мастера — отдельно: стем master и master-glue без стемов, у них своя секция', () => {
    const ids = masterPresets().map((p) => p.id)
    expect(ids).toEqual(expect.arrayContaining(['master-stream', 'master-loud', 'master-soft-glue', 'master-glue']))
    for (const target of EDITOR_TARGETS) {
      expect(idsOf(target).some((id) => ids.includes(id))).toBe(false)
    }
  })

  it('у барабанов — комната и наборы по частям (замена сэмплами), синтов нет', () => {
    expect(idsOf('drums')).toContain('drums-room')
    expect(idsOf('drums')).toContain('drums-kick-kit')
    expect(idsOf('drums')).toContain('drums-hh-kit')
    expect(idsOf('drums').some((id) => id.startsWith('synth-'))).toBe(false)
  })

  it('гитара и клавиши: свои группы, соседние дорожки не подтекают', () => {
    expect(idsOf('guitar')).toContain('guitar-amp-clean')
    expect(idsOf('guitar')).not.toContain('bass-melodic')
    expect(idsOf('piano')).toContain('synth-delay')   // стемы ['other','piano'] — подходит обеим
    expect(idsOf('other')).toContain('synth-delay')
  })

  it('партийные пресеты (первый блок synth/perc) не подходят ни одному табу', () => {
    const parted = fxPresets.filter(isPartItem)
    expect(parted.length).toBeGreaterThan(10)
    for (const p of parted) {
      for (const target of EDITOR_TARGETS) expect(itemForTarget(p, target), p.id).toBe(false)
    }
  })

  it('мастер не попадает ни в один таб', () => {
    for (const target of EDITOR_TARGETS) {
      expect(idsOf(target)).not.toContain('master-stream')
    }
  })
})

describe('specStemsFor — дефолт дорожек записи из каталога', () => {
  const byId = (id) => fxPresets.find((p) => p.id === id)

  it('гитарный пресет на табе гитары — стем guitar (не guitar+other), как во встроенных', () => {
    expect(specStemsFor(byId('guitar-clean'), 'guitar')).toEqual(['guitar'])
  })

  it('набор бочки на табе барабанов — стем kick, комната — drums', () => {
    expect(specStemsFor(byId('drums-kick-kit'), 'drums')).toEqual(['kick'])
    expect(specStemsFor(byId('drums-room'), 'drums')).toEqual(['drums'])
  })

  it('универсальный пресет (без стемов) — дорожка таба', () => {
    expect(specStemsFor({ stems: [] }, 'bass')).toEqual(['bass'])
  })

  it('дилей синта (other+piano) на табе клавиш — только piano', () => {
    expect(specStemsFor(byId('synth-delay'), 'piano')).toEqual(['piano'])
  })

  it('specFromItem: запись движка с uid, нейтральное место и без цели громкости не пишутся', () => {
    const sp = specFromItem(byId('bass-fuzz'), 'bass')
    expect(sp).toMatchObject({ stems: ['bass'], db: 0 })
    expect(sp.engine.map((b) => b.type)).toEqual(['drive', 'eq', 'comp'])
    expect(sp.uid).toBeTruthy()
    expect(bodyFromModel({ ...newModel(), specs: [sp] }).specs[0]).not.toHaveProperty('uid')
    expect(bodyFromModel({ ...newModel(), specs: [sp] }).specs[0]).not.toHaveProperty('place')
  })

  it('specFromItem подставляет захват усилителю без захвата по подсказке пресета', () => {
    const amps = [{ name: 'Bug333' }, { name: 'Fender Twin 65' }]
    const sp = specFromItem(byId('guitar-amp-clean'), 'guitar', amps)
    const amp = sp.engine.find((b) => b.type === 'amp')
    expect(amp.model).toBe('Fender Twin 65')   // amp_hint пресета, не первый по алфавиту
  })
})

describe('specKindOk — на частях барабанов только цепочка движка', () => {
  it('эффект и педали на kick/hh — нельзя, движок и «только громкость» — можно', () => {
    expect(specKindOk(['kick'], 'chain')).toBe(false)
    expect(specKindOk(['hh', 'snare'], 'steps')).toBe(false)
    expect(specKindOk(['kick'], 'engine')).toBe(true)
    expect(specKindOk(['kick'], '')).toBe(true)
    expect(specKindOk(['bass'], 'chain')).toBe(true)
    expect(specKindOk(['drums'], 'steps')).toBe(true)
  })

  it('specKind понимает все виды записи', () => {
    expect(specKind(FULL.specs[0])).toBe('engine')
    expect(specKind(FULL.specs[1])).toBe('chain')
    expect(specKind(FULL.specs[2])).toBe('steps')
    expect(specKind(FULL.specs[3])).toBe('')
  })
})

describe('previewSource — дорожка предпрослушивания', () => {
  const basic = new Set(['vocals', 'drums', 'bass', 'other'])
  const roformer = new Set([...basic, 'guitar', 'piano', 'kick', 'snare', 'toms', 'hh', 'ride', 'crash'])

  it('основная дорожка из записи, которая есть у эталона (гитары нет — слушаем other)', () => {
    expect(previewSource(['guitar', 'other'], basic)).toEqual({ source: 'other' })
    expect(previewSource(['guitar', 'other'], roformer)).toEqual({ source: 'guitar' })
  })

  it('часть барабанов: есть у эталона — слушаем её (первая в записи), нет — честный blocked про RoFormer', () => {
    expect(previewSource(['kick'], roformer)).toEqual({ source: 'kick' })
    expect(previewSource(['hh', 'kick'], roformer)).toEqual({ source: 'kick' })   // порядок записи, не алфавит
    expect(previewSource(['kick'], basic)).toEqual({ blocked: 'roformer' })
  })

  it('дорожек у эталона ещё нет — nostems (их предстоит сделать)', () => {
    expect(previewSource(['bass'], new Set())).toEqual({ blocked: 'nostems' })
    expect(previewSource(['bass'], new Set(['drums']))).toEqual({ blocked: 'nostems' })
  })
})

describe('toggleStem — чипы дорожек записи', () => {
  it('добавляет и убирает; последнюю дорожку убрать нельзя', () => {
    expect(toggleStem(['bass'], 'drums')).toEqual(['bass', 'drums'])
    expect(toggleStem(['bass', 'drums'], 'bass')).toEqual(['drums'])
    expect(toggleStem(['bass'], 'bass')).toEqual(['bass'])
  })
})

describe('rhythmSpecs — «ритм-секция набором» в записи пресета', () => {
  it('записи только по дорожкам, которые есть у эталона: {stems:[часть], engine}, бас — тоже', () => {
    const out = rhythmSpecs(new Set(['kick', 'snare', 'hh', 'bass', 'other']), 'dry', 'osdk')
    expect(out.map((s) => s.stems)).toEqual([['kick'], ['snare'], ['hh'], ['bass']])
    expect(out.every((s) => Array.isArray(s.engine) && s.engine.length && specKindOk(s.stems, specKind(s)))).toBe(true)
  })

  it('набор машины — sampler с набором машины, обработка дописана в цепочку', () => {
    const out = rhythmSpecs(new Set(['kick', 'bass']), 'gated', 'tr808')
    const kick = out.find((s) => s.stems.includes('kick'))
    expect(kick.engine[0]).toMatchObject({ type: 'sampler', kit: 'tr808/kick' })
    expect(kick.engine[kick.engine.length - 1].type).not.toBe('sampler')   // обработка дописана
  })
})

describe('EDIT_TARGETS и семьи демо', () => {
  it('табы — известные дорожки, части барабанов — отдельным списком', () => {
    expect(EDITOR_TARGETS).toEqual(['vocals', 'drums', 'bass', 'guitar', 'piano', 'other'])
    expect(DRUM_PARTS).toEqual(['kick', 'snare', 'toms', 'hh', 'ride', 'crash'])
  })

  it('семьи пресетов не пусты (демо-эталоны строятся по ним)', () => {
    expect(PRESET_FAMILIES.length).toBeGreaterThan(0)
  })
})

// ТК157 (128е): превью мастера в редакторе = то, что применит Go presetMaster (internal/studio/presets.go):
// нет цели — цепочка как есть (ограничитель и его цель не трогаются); цель есть — уходит в ограничитель
// с target_lufs 0/без цели, ненулевую цель ограничителя не перебивает; нет ограничителя — он дописывается
// в конец. Входной массив и его блоки не мутируются.
describe('presetMasterChain — мастер пресета как в Go presetMaster', () => {
  const GLUE = { type: 'glue', ratio: 2 }

  it('нет цели (null/undefined) — копия цепочки как есть, ограничитель остаётся со своей целью', () => {
    const chain = [GLUE, { type: 'limiter', target_lufs: -9, ceiling_db: -1 }]
    for (const t of [null, undefined]) {
      const out = presetMasterChain(chain, t)
      expect(out, String(t)).toEqual(chain)
      expect(out).not.toBe(chain)
    }
    // ограничитель без цели при пустой цели пресета ничего не получает
    const zero = [{ type: 'limiter', target_lufs: 0 }]
    expect(presetMasterChain(zero, null)).toEqual([{ type: 'limiter', target_lufs: 0 }])
  })

  it('нет цели и пустая цепочка — пустой мастер (ограничитель не дописывается)', () => {
    expect(presetMasterChain([], null)).toEqual([])
  })

  it('цель + ограничитель с target_lufs 0 — ограничитель получает цель, место и прочие поля прежние', () => {
    const out = presetMasterChain([{ type: 'limiter', target_lufs: 0, ceiling_db: -1 }, GLUE], -14)
    expect(out).toEqual([{ type: 'limiter', target_lufs: -14, ceiling_db: -1 }, GLUE])
  })

  it('цель + ограничитель без поля target_lufs — тоже получает цель (как 0 в Go)', () => {
    expect(presetMasterChain([GLUE, { type: 'limiter', ceiling_db: -1 }], -12))
      .toEqual([GLUE, { type: 'limiter', ceiling_db: -1, target_lufs: -12 }])
  })

  it('цель + ограничитель со своей ненулевой целью — не меняется и второй ограничитель не появляется', () => {
    const chain = [GLUE, { type: 'limiter', target_lufs: -9 }]
    expect(presetMasterChain(chain, -14)).toEqual([GLUE, { type: 'limiter', target_lufs: -9 }])
  })

  it('цель без ограничителя — ограничитель с целью дописан в конец', () => {
    expect(presetMasterChain([GLUE, { type: 'eq', low_db: 1 }], -14))
      .toEqual([GLUE, { type: 'eq', low_db: 1 }, { type: 'limiter', target_lufs: -14 }])
    expect(presetMasterChain([], -16)).toEqual([{ type: 'limiter', target_lufs: -16 }])
  })

  it('цель 0 (не пусто) — как в Go: ограничитель с целью 0 дописан, 0 не равен «нет цели»', () => {
    expect(presetMasterChain([GLUE], 0)).toEqual([GLUE, { type: 'limiter', target_lufs: 0 }])
  })

  it('несколько ограничителей — цель получает последний (как в Go)', () => {
    const out = presetMasterChain([{ type: 'limiter', target_lufs: 0 }, GLUE, { type: 'limiter', target_lufs: 0 }], -14)
    expect(out).toEqual([{ type: 'limiter', target_lufs: 0 }, GLUE, { type: 'limiter', target_lufs: -14 }])
  })

  it('входной массив и его блоки не мутируются; правка результата не задевает вход', () => {
    const chain = [GLUE, { type: 'limiter', target_lufs: 0, ceiling_db: -1 }]
    const before = JSON.parse(JSON.stringify(chain))
    const out = presetMasterChain(chain, -14)
    expect(chain).toEqual(before)
    out[0].ratio = 99
    out.push({ type: 'x' })
    expect(chain).toEqual(before)
    const asIs = presetMasterChain(chain, null)
    asIs[1].target_lufs = -3
    expect(chain).toEqual(before)
  })
})

// ТК157 (127д): записи «ритм-секции набором» — у каждой свой непустой uid (ключ списка и плеера)
describe('rhythmSpecs — свой uid у каждой записи', () => {
  it('uid непустой и не повторяется внутри набора и между двумя наборами в одном пресете', () => {
    const avail = new Set(['kick', 'snare', 'hh', 'bass'])
    const a = rhythmSpecs(avail, 'dry', 'osdk')
    const b = rhythmSpecs(avail, 'dry', 'osdk')
    expect(a.length).toBe(4)
    for (const s of [...a, ...b]) expect(typeof s.uid === 'string' || typeof s.uid === 'number', JSON.stringify(s)).toBe(true)
    for (const s of [...a, ...b]) expect(String(s.uid).length).toBeGreaterThan(0)
    const uids = [...a, ...b].map((s) => s.uid)
    expect(new Set(uids).size).toBe(uids.length)
    // uid служебный: в тело сохранения не попадает
    for (const s of bodyFromModel({ ...newModel(), specs: a }).specs) expect(s).not.toHaveProperty('uid')
  })
})

// ТК158 (127е): снимок для «есть несохранённые правки» — правка тонкой цепочки (рабочая форма edit)
// видна, id пресета не в счёт, мастер входит, модель не меняется.
describe('editorSnapshot — снимок состояния редактора', () => {
  const ENGINE = [{ type: 'comp', threshold_db: -20, ratio: 4 }]
  const base = () => toModel({ id: 5, name: 'п', specs: [{ stems: ['bass'], engine: ENGINE, db: 0 }] })
  const MASTER = [{ type: 'limiter', target_lufs: -14 }]

  it('модели, отличающиеся только id, дают одинаковый снимок', () => {
    const m = base()
    expect(editorSnapshot({ ...m, id: 99 }, MASTER)).toEqual(editorSnapshot(m, MASTER))
    expect(editorSnapshot({ ...m, id: 0 }, MASTER)).toEqual(editorSnapshot(m, MASTER))
  })

  it('изменённый параметр блока в рабочей форме edit — снимок отличается от модели без правки', () => {
    const plain = base()
    const edited = JSON.parse(JSON.stringify(plain))
    const sp = edited.specs[0]
    sp.edit = setParam(fromWorkerChain(sp.engine, BLOCKS), 0, 'threshold_db', -35, BLOCKS)
    expect(sp.edit[0].params.threshold_db).toBe(-35)   // правка действительно внесена
    expect(editorSnapshot(edited, MASTER)).not.toEqual(editorSnapshot(plain, MASTER))
  })

  it('разные значения правки в edit — разные снимки (учитывается значение, а не факт наличия edit)', () => {
    const a = JSON.parse(JSON.stringify(base()))
    const b = JSON.parse(JSON.stringify(a))
    a.specs[0].edit = setParam(fromWorkerChain(ENGINE, BLOCKS), 0, 'ratio', 8, BLOCKS)
    b.specs[0].edit = setParam(fromWorkerChain(ENGINE, BLOCKS), 0, 'ratio', 2, BLOCKS)
    expect(editorSnapshot(a, MASTER)).not.toEqual(editorSnapshot(b, MASTER))
  })

  it('edit, раскрытый без правок, — повторный снимок равен себе (детерминирован)', () => {
    const m = base()
    m.specs[0].edit = fromWorkerChain(m.specs[0].engine, BLOCKS)
    const first = editorSnapshot(m, MASTER)
    expect(editorSnapshot(m, MASTER)).toEqual(first)
    expect(editorSnapshot(JSON.parse(JSON.stringify(m)), MASTER)).toEqual(first)
  })

  it('master входит в снимок: другой master — другой снимок, пустой master — тоже отличие', () => {
    const m = base()
    const other = [{ type: 'limiter', target_lufs: -9 }]
    expect(editorSnapshot(m, other)).not.toEqual(editorSnapshot(m, MASTER))
    expect(editorSnapshot(m, [])).not.toEqual(editorSnapshot(m, MASTER))
  })

  it('снимок не мутирует модель и master', () => {
    const m = base()
    m.specs[0].edit = setParam(fromWorkerChain(m.specs[0].engine, BLOCKS), 0, 'threshold_db', -30, BLOCKS)
    const master = JSON.parse(JSON.stringify(MASTER))
    const before = JSON.parse(JSON.stringify(m))
    editorSnapshot(m, master)
    expect(m).toEqual(before)
    expect(master).toEqual(MASTER)
  })

  it('пустая модель (newModel) — снимок строится, правка имени его меняет', () => {
    const m = newModel()
    const s0 = editorSnapshot(m, [])
    expect(s0).toBeDefined()
    expect(editorSnapshot({ ...m, name: 'другое' }, [])).not.toEqual(s0)
  })
})
