package studio

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"yue-studio/internal/dsp"
	"yue-studio/internal/yue"
)

// Импорт рецепта трека: ведущий проигрывает рецепт тем же кодом, что делал трек, —
// корень генерацией, затем записи журнала по порядку, с новыми id треков и именами файлов. Состояние
// сессии — на воркере: перезапуск приложения продолжает с первого незавершённого шага, готовые не переделываются.

// Recipe — рецепт как его отдаёт воркер (GET /jobs/{id}/recipe).
type Recipe struct {
	Format      string             `json:"format"`
	Version     int                `json:"version"`
	WorkerModel string             `json:"worker_model"`
	StemsModel  string             `json:"stems_model"`
	Track       RecipeTrack        `json:"track"`
	Root        RecipeRoot         `json:"root"`
	Ops         []RecipeOp         `json:"ops"`
	Durations   map[string]float64 `json:"durations"`
	Warnings    []string           `json:"warnings"`
}

type RecipeTrack struct {
	Title  string `json:"title"`
	Folder string `json:"folder"`
	Head   string `json:"head"`
}

type RecipeRoot struct {
	Title       string  `json:"title"`
	Style       string  `json:"style"`
	Lyrics      string  `json:"lyrics"`
	Seed        int64   `json:"seed"`
	Cot         string  `json:"cot"`
	Arc         string  `json:"arc"`
	MaxTokens   int64   `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Cfg         float64 `json:"cfg"`
	Abc         *string `json:"abc"`
	PlanAbc     *string `json:"plan_abc"`
}

type RecipeOp struct {
	Op     string          `json:"op"`
	Node   string          `json:"node"`
	Params json.RawMessage `json:"params"`
	Result string          `json:"result"`
	Refs   []string        `json:"refs"`
}

// recipeState — ход импорта: Next — индекс следующей записи (-1 — корень ещё не создан), Waiting — джоба,
// которую ждём (WaitNode — её узел), Nodes — n-id → новый id, Files — "<узел>/<старый файл>" → новый файл.
type recipeState struct {
	Next     int               `json:"next"`
	Waiting  int64             `json:"waiting"`
	WaitNode string            `json:"wait_node,omitempty"`
	Nodes    map[string]int64  `json:"nodes"`
	Files    map[string]string `json:"files"`
	Notes    []string          `json:"notes,omitempty"`
}

// RecipeRunner — ведущий импорта: приложение зовёт Tick раз в несколько секунд, MCP — свой цикл. Один шаг на
// тик; сессию держит тот, кто её захватил (ClaimRecipeImport), второй ведущий ждёт.
type RecipeRunner struct {
	Svc   yue.Service
	Owner string // уникален на экземпляр ведущего (RecipeOwner): одинаковый у двух — захват их не различит

	mu      sync.Mutex
	unsaved map[int64]unsavedStep // шаг сделан, ход не сохранён: досохранить, а не повторять шаг
	base    map[int64]string      // состояние сессии на воркере в начале текущего шага
}

// unsavedStep — несохранённый ход и состояние, от которого шаг начинался: на воркере уже другое — значит,
// импорт вёл другой ведущий, и старый ход его прогресс не перезаписывает.
type unsavedStep struct {
	u    yue.RecipeImportUpdate
	base string
}

// RecipeOwner — имя ведущего: программа, машина, процесс и случайная часть (PID на разных машинах совпадает).
func RecipeOwner(kind string) string {
	host, _ := os.Hostname()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s-%d-%x", kind, host, os.Getpid(), b)
}

// Tick — один шаг первой незавершённой сессии; true — шаг сделан сразу (правка или готовая генерация) и
// следующий можно делать без паузы; false — нечего делать, ждём генерацию или сессию ведёт другой.
func (r *RecipeRunner) Tick(ctx context.Context) bool {
	list, err := r.Svc.RecipeImports(ctx)
	if err != nil {
		return false
	}
	for _, imp := range list {
		if imp.Status != "running" {
			continue
		}
		if err := r.Svc.ClaimRecipeImport(ctx, imp.ID, r.Owner); err != nil {
			if !errors.Is(err, yue.ErrClaimed) {
				log.Printf("импорт рецепта #%d: захват: %v", imp.ID, err)
			}
			continue
		}
		full, err := r.Svc.RecipeImport(ctx, imp.ID)
		if err != nil {
			log.Printf("импорт рецепта #%d: %v", imp.ID, err)
			return false
		}
		switch r.flushUnsaved(ctx, imp.ID, string(full.State)) {
		case flushSaved:
			return true // прошлый шаг досохранён — следующий тиком
		case flushPending, flushClaimed:
			return false // воркер не принимает сохранение (шаг не повторяем) или импорт перехватили
		}
		r.mu.Lock()
		if r.base == nil {
			r.base = map[int64]string{}
		}
		r.base[imp.ID] = string(full.State)
		r.mu.Unlock()
		stop := r.holdLease(ctx, imp.ID)
		defer stop()
		return r.step(ctx, full)
	}
	return false
}

func strp(s string) *string { return &s }

// RecipeImportAction — действие человека над упавшим импортом: retry — снова в работу с того же шага
// (готовые шаги не переделываются), dismiss — убрать из списка над треками.
func RecipeImportAction(ctx context.Context, svc yue.Service, id int64, action string) error {
	var u yue.RecipeImportUpdate
	switch action {
	case "retry":
		u = yue.RecipeImportUpdate{Status: strp("running"), Error: strp("")}
	case "dismiss":
		u = yue.RecipeImportUpdate{Status: strp("dismissed")}
	default:
		return fmt.Errorf("действие %q: retry или dismiss", action)
	}
	_, err := svc.SaveRecipeImport(ctx, id, u)
	return err
}

// save — состояние сессии; без отмены контекста: шаг уже сделан на воркере, закрытие приложения посреди
// сохранения иначе повторило бы его после перезапуска. Импорт перехватил другой — его состояние главнее.
// false — сессию перехватил другой ведущий (или воркер не принял): дальше этот тик не идёт.
func (r *RecipeRunner) save(ctx context.Context, id int64, st *recipeState, u yue.RecipeImportUpdate) bool {
	u.State, u.Owner = yue.Params(st), r.Owner
	if _, err := r.Svc.SaveRecipeImport(context.WithoutCancel(ctx), id, u); err != nil {
		log.Printf("импорт рецепта #%d: сохранение: %v", id, err)
		if !errors.Is(err, yue.ErrClaimed) { // перехватил другой — его ход главнее; иначе досохраним позже
			r.mu.Lock()
			if r.unsaved == nil {
				r.unsaved = map[int64]unsavedStep{}
			}
			r.unsaved[id] = unsavedStep{u: u, base: r.base[id]}
			r.mu.Unlock()
		}
		return false
	}
	return true
}

const (
	flushNone    = iota // нечего досохранять
	flushSaved          // досохранено
	flushPending        // воркер всё ещё не принимает — ждём, шаг не повторяем
	flushClaimed        // ход устарел (импорт вёл другой) или перехвачен — выброшен
)

// flushUnsaved — досохранить ход шага, сделанного раньше. current — состояние на воркере сейчас: если оно не то,
// от которого шаг начинался, импорт за это время вёл другой — старый ход выбрасывается, не перезаписывая его.
func (r *RecipeRunner) flushUnsaved(ctx context.Context, id int64, current string) int {
	r.mu.Lock()
	p, ok := r.unsaved[id]
	r.mu.Unlock()
	if !ok {
		return flushNone
	}
	drop := func() {
		r.mu.Lock()
		delete(r.unsaved, id)
		r.mu.Unlock()
	}
	if p.base != current {
		drop()
		log.Printf("импорт рецепта #%d: несохранённый ход устарел (импорт вёл другой) — выброшен", id)
		return flushClaimed
	}
	_, err := r.Svc.SaveRecipeImport(context.WithoutCancel(ctx), id, p.u)
	switch {
	case err == nil:
		drop()
		return flushSaved
	case errors.Is(err, yue.ErrClaimed):
		drop()
		return flushClaimed
	}
	log.Printf("импорт рецепта #%d: досохранение: %v", id, err)
	return flushPending
}

// recipeLeaseEvery — как часто ведущий продлевает захват во время шага (лиза воркера — 600 с): долгий шаг
// (пресет, эффект в очереди видеокарты) иначе перехватил бы второй ведущий и повторил его
var recipeLeaseEvery = 60 * time.Second // var — тесты подменяют

// holdLease — продлевать захват, пока не вызван stop (шаг закончен).
func (r *RecipeRunner) holdLease(ctx context.Context, id int64) (stop func()) {
	done, exited := make(chan struct{}), make(chan struct{})
	every := recipeLeaseEvery
	go func() {
		defer close(exited)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if err := r.Svc.ClaimRecipeImport(context.WithoutCancel(ctx), id, r.Owner); err != nil {
					log.Printf("импорт рецепта #%d: продление захвата: %v", id, err)
				}
			}
		}
	}()
	return func() { close(done); <-exited } // stop возвращается, когда продления уже не будет
}

func (r *RecipeRunner) step(ctx context.Context, imp *yue.RecipeImport) bool {
	var rec Recipe
	if err := json.Unmarshal(imp.Recipe, &rec); err != nil {
		r.save(ctx, imp.ID, &recipeState{}, yue.RecipeImportUpdate{Status: strp("error"), Error: strp("рецепт не читается: " + err.Error())})
		return false
	}
	st := recipeState{Next: -1}
	if len(imp.State) > 0 && string(imp.State) != "{}" {
		if err := json.Unmarshal(imp.State, &st); err != nil {
			st = recipeState{Next: -1}
		}
	}
	if st.Nodes == nil {
		st.Nodes = map[string]int64{}
	}
	if st.Files == nil {
		st.Files = map[string]string{}
	}
	total := len(rec.Ops) + 1

	if st.Waiting != 0 {
		return r.wait(ctx, imp, &rec, &st, total)
	}
	if st.Next < 0 {
		id, err := r.Svc.Submit(ctx, rootParams(rec.Root, imp.Mode))
		if err != nil {
			r.fail(ctx, imp.ID, &st, "корень: "+err.Error())
			return false
		}
		st.Waiting, st.WaitNode = id, "n1"
		r.save(ctx, imp.ID, &st, yue.RecipeImportUpdate{Progress: strp(fmt.Sprintf("шаг 1 из %d: генерация трека #%d", total, id)),
			RootJobID: &id})
		return false
	}
	if st.Next >= len(rec.Ops) {
		r.finish(ctx, imp, &rec, &st)
		return false
	}
	op := rec.Ops[st.Next]
	progress := fmt.Sprintf("шаг %d из %d: %s", st.Next+2, total, opLabel(op.Op))
	waitID, err := r.apply(ctx, &rec, &st, op)
	if err != nil {
		r.fail(ctx, imp.ID, &st, progress+": "+err.Error())
		return false
	}
	if waitID != 0 {
		st.Waiting, st.WaitNode = waitID, strings.TrimPrefix(op.Result, "job:")
		progress += fmt.Sprintf(" — генерация #%d", waitID)
	} else {
		st.Next++
	}
	return r.save(ctx, imp.ID, &st, yue.RecipeImportUpdate{Progress: &progress}) && waitID == 0
}

// wait — генерация, которую ждём: готова — узел записан, следующий шаг; ошибка/отмена — импорт с ошибкой.
func (r *RecipeRunner) wait(ctx context.Context, imp *yue.RecipeImport, rec *Recipe, st *recipeState, total int) bool {
	jobs, err := r.Svc.Jobs(ctx)
	if err != nil {
		return false // воркер недоступен — следующий тик
	}
	for _, j := range jobs {
		if j.ID != st.Waiting {
			continue
		}
		switch j.Status {
		case "done":
			st.Nodes[st.WaitNode] = j.ID
			if want := rec.Durations[st.WaitNode]; want > 0 && j.DurationSec > 0 && math.Abs(j.DurationSec-want)/want > 0.02 {
				st.Notes = append(st.Notes, fmt.Sprintf("%s: длительность %.1f с вместо %.1f с — окна правок могли сместиться",
					st.WaitNode, j.DurationSec, want))
			}
			st.Waiting, st.WaitNode = 0, ""
			st.Next++
			progress := fmt.Sprintf("шаг %d из %d готов", min(st.Next+1, total), total)
			if len(st.Notes) > 0 {
				progress += " · " + strings.Join(st.Notes, "; ")
			}
			return r.save(ctx, imp.ID, st, yue.RecipeImportUpdate{Progress: &progress})
		case "error", "canceled":
			msg := fmt.Sprintf("генерация #%d: %s", j.ID, j.Status)
			if j.Error != "" {
				msg += " — " + j.Error
			}
			st.Waiting, st.WaitNode = 0, ""
			r.fail(ctx, imp.ID, st, msg)
		}
		return false
	}
	st.Waiting, st.WaitNode = 0, ""
	r.fail(ctx, imp.ID, st, "генерация пропала из списка треков (удалена?)")
	return false
}

func (r *RecipeRunner) fail(ctx context.Context, id int64, st *recipeState, msg string) {
	r.save(ctx, id, st, yue.RecipeImportUpdate{Status: strp("error"), Error: &msg})
}

// finish — основная версия и папка как в рецепте, сессия готова.
func (r *RecipeRunner) finish(ctx context.Context, imp *yue.RecipeImport, rec *Recipe, st *recipeState) {
	root := st.Nodes["n1"]
	// основная — как в рецепте, и когда это сам трек: журнал мог переиграть давний выбор другой версии
	if head := st.Nodes[rec.Track.Head]; head != 0 {
		if head == root {
			head = 0
		}
		if err := r.Svc.SetHead(ctx, root, head); err != nil {
			r.fail(ctx, imp.ID, st, "основная версия: "+err.Error())
			return
		}
	}
	if rec.Track.Folder != "" {
		if _, err := r.Svc.UpdateJob(ctx, root, nil, &rec.Track.Folder); err != nil {
			r.fail(ctx, imp.ID, st, "папка: "+err.Error())
			return
		}
	}
	progress := "готово"
	if len(st.Notes) > 0 {
		progress += " · " + strings.Join(st.Notes, "; ")
	}
	r.save(ctx, imp.ID, st, yue.RecipeImportUpdate{Status: strp("done"), Progress: &progress, RootJobID: &root})
}

// rootParams — корень песни: по сиду — та же заявка (тот же путь, что у оригинала); по плану — рендер по
// плану нот оригинала (модель другой версии: ноты те же, исполнение новое).
func rootParams(root RecipeRoot, mode string) yue.SubmitParams {
	p := yue.SubmitParams{Title: root.Title, Style: root.Style, Lyrics: root.Lyrics, Seed: root.Seed, Cot: root.Cot,
		Arc: root.Arc, MaxTokens: root.MaxTokens, Temperature: root.Temperature, Cfg: root.Cfg}
	if root.Abc != nil {
		p.Abc = *root.Abc
	}
	if mode == "plan" && root.PlanAbc != nil && *root.PlanAbc != "" {
		p.Abc, p.Cot, p.Arc = *root.PlanAbc, "melody", "" // драматургия уже в плане
	}
	if p.Cot == "" {
		p.Cot = "full"
	}
	return p
}

var opLabels = map[string]string{
	"submit": "рендер версии", "continue": "продолжение с места", "fragment": "кусок", "overdub": "переиграть",
	"variant_track": "версия из варианта", "fx": "эффект", "minus": "минус", "update": "название/папка",
	"set_head": "основная версия", "rebuild_sections": "пересборка дорожек", "envelope": "линия громкости",
	"splice": "склейка", "dsp_apply": "эффект", "pedals": "педали", "sound_preset": "пресет звука",
}

func opLabel(op string) string {
	if l, ok := opLabels[op]; ok {
		return l
	}
	return op
}

// ключи params с id треков (как в worker/recipe.py ID_KEYS)
var recipeIDKeys = map[string]bool{"parent_id": true, "child_id": true, "job_id": true, "voice_src": true, "head_id": true}

var nodeRef = regexp.MustCompile(`\{(n\d+)\}`)

// remap — params записи с новыми id треков (n-id → id) и именами файлов (старые результаты узла → новые,
// overdub-{nK}.flac → overdub-<новый id>.flac). Неизвестный узел — ошибка: шаги идут по порядку, ссылка
// вперёд значит испорченный рецепт.
func remap(v any, node string, st *recipeState) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if s, ok := val.(string); ok && recipeIDKeys[k] && strings.HasPrefix(s, "n") {
				id, ok := st.Nodes[s]
				if !ok {
					return nil, fmt.Errorf("ссылка на узел %s, которого ещё нет", s)
				}
				out[k] = id
				continue
			}
			m, err := remap(val, node, st)
			if err != nil {
				return nil, err
			}
			out[k] = m
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			m, err := remap(val, node, st)
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	case string:
		if f, ok := st.Files[node+"/"+x]; ok {
			return f, nil
		}
		var missing string
		s := nodeRef.ReplaceAllStringFunc(x, func(m string) string {
			n := m[1 : len(m)-1]
			id, ok := st.Nodes[n]
			if !ok {
				missing = n
				return m
			}
			return strconv.FormatInt(id, 10)
		})
		if missing != "" {
			return nil, fmt.Errorf("ссылка на узел %s, которого ещё нет", missing)
		}
		return s, nil
	}
	return v, nil
}

// decode — params записи после remap в структуру операции.
func decode(op RecipeOp, st *recipeState, out any) error {
	var raw any
	if len(op.Params) > 0 {
		if err := json.Unmarshal(op.Params, &raw); err != nil {
			return fmt.Errorf("параметры: %w", err)
		}
	}
	m, err := remap(raw, op.Node, st)
	if err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// apply — одна запись журнала на новых треках. Генерация — id новой джобы (ждать); правка — сразу, её файл
// записывается в Files (старое имя → новое).
func (r *RecipeRunner) apply(ctx context.Context, rec *Recipe, st *recipeState, op RecipeOp) (int64, error) {
	target, ok := st.Nodes[op.Node]
	if !ok {
		return 0, fmt.Errorf("узел %s ещё не создан", op.Node)
	}
	svc := r.Svc
	file := func(newFile string) {
		if op.Result != "" && newFile != "" {
			st.Files[op.Node+"/"+resultName(op.Result, st)] = newFile
			st.Files[op.Node+"/"+op.Result] = newFile
		}
	}
	switch op.Op {
	case "submit":
		var p yue.SubmitParams
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		p.SoundPresetIDs = nil // пресеты придут своими записями sound_preset
		return svc.Submit(ctx, p)
	case "continue", "fragment":
		var p struct {
			FromSec  float64 `json:"from_sec"`
			ToSec    float64 `json:"to_sec"`
			Seed     int64   `json:"seed"`
			Abc      string  `json:"abc"`
			StyleAdd string  `json:"style_add"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		if op.Op == "fragment" {
			return svc.FragmentJob(ctx, target, p.FromSec, p.ToSec, p.Seed, p.Abc, p.StyleAdd)
		}
		return svc.ContinueJob(ctx, target, p.FromSec, p.Seed, p.Abc, p.StyleAdd)
	case "overdub":
		var p struct {
			Style  string  `json:"style"`
			Lyrics string  `json:"lyrics"`
			Gain   float64 `json:"gain"`
			Abc    string  `json:"abc"`
			Seed   int64   `json:"seed"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		return svc.SubmitOverdub(ctx, target, p.Style, p.Lyrics, p.Gain, p.Abc, p.Seed)
	case "variant_track":
		var p struct {
			File     string `json:"file"`
			Title    string `json:"title"`
			VoiceSrc int64  `json:"voice_src"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		id, err := svc.VariantToTrack(ctx, target, p.File, p.Title, p.VoiceSrc)
		if err != nil {
			return 0, err
		}
		st.Nodes[strings.TrimPrefix(op.Result, "job:")] = id // версия готова сразу
		return 0, nil
	case "sound_preset":
		var p struct {
			Preset yue.SoundPreset `json:"preset"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		res, err := ApplySoundPreset(ctx, svc, target, p.Preset)
		if err != nil {
			return 0, err
		}
		st.Nodes[strings.TrimPrefix(op.Result, "job:")] = res.ChildID
		return 0, nil
	case "fx":
		var p yue.FxRequest
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		v, err := svc.ApplyFx(ctx, target, p)
		if err != nil {
			return 0, err
		}
		file(v.File)
	case "minus":
		var p struct {
			Exclude []string `json:"exclude"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		out, err := svc.MakeMinus(ctx, target, p.Exclude)
		if err != nil {
			return 0, err
		}
		f, _ := out["file"].(string)
		file(f)
	case "update":
		var p struct {
			Title  *string `json:"title"`
			Folder *string `json:"folder"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		if _, err := svc.UpdateJob(ctx, target, p.Title, p.Folder); err != nil {
			return 0, err
		}
	case "set_head":
		var p struct {
			HeadID int64 `json:"head_id"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		if err := svc.SetHead(ctx, target, p.HeadID); err != nil {
			return 0, err
		}
	case "rebuild_sections":
		var p struct {
			Specs []SectionSpec `json:"specs"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		res, err := RebuildSections(ctx, svc, target, p.Specs)
		if err != nil {
			return 0, err
		}
		if res.Variant != nil {
			file(res.Variant.File)
		}
	case "envelope":
		var p struct {
			Stem   string         `json:"stem"`
			Points []dsp.EnvPoint `json:"points"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		v, err := VolumeEnvelope(ctx, svc, target, p.Stem, p.Points)
		if err != nil {
			return 0, err
		}
		file(v.File)
	case "splice":
		var p struct {
			Parts     []SplicePart `json:"parts"`
			Crossfade float64      `json:"crossfade"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		v, err := Splice(ctx, svc, target, p.Parts, p.Crossfade)
		if err != nil {
			return 0, err
		}
		file(v.File)
	case "dsp_apply":
		var p struct {
			Chain  string             `json:"chain"`
			Params map[string]float64 `json:"params"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		v, err := ApplyChain(ctx, svc, target, p.Chain, p.Params)
		if err != nil {
			return 0, err
		}
		file(v.File)
	case "pedals":
		var p struct {
			Steps []dsp.Step `json:"steps"`
			Label string     `json:"label"`
		}
		if err := decode(op, st, &p); err != nil {
			return 0, err
		}
		v, err := ApplySteps(ctx, svc, target, p.Steps, p.Label)
		if err != nil {
			return 0, err
		}
		file(v.File)
	default:
		return 0, fmt.Errorf("неизвестная операция %q", op.Op)
	}
	return 0, nil
}

// resultName — старое имя файла-результата с подставленными id узлов (overdub-inst-{n3}.flac → …-<id>.flac):
// под этим именем на него могут сослаться записи, сделанные до импорта узла.
func resultName(result string, st *recipeState) string {
	return nodeRef.ReplaceAllStringFunc(result, func(m string) string {
		if id, ok := st.Nodes[m[1:len(m)-1]]; ok {
			return strconv.FormatInt(id, 10)
		}
		return m
	})
}
