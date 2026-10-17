package studio

import (
	"context"
	"log"
	"math"
	"slices"
	"time"

	"partscaster/internal/yue"
)

// Сверка баланса итога: собранный трек делится на дорожки и меряется как эталоны; записи с целью (VerifyDb), которые
// промахнулись, поправляются по Db, трек собирается заново. Общая для «применить всё» черновика и пресета сразу в
// трек: цель ставится до мастера, а мастер и остатки старых дорожек сдвигают баланс (#725: гитара −5,9 при цели
// −9,5); один проход исправлял ~половину (#728: −5,9 → −7,2), поэтому их до verifyPasses.

// verifyTolDb — промах цели больше этого — исправляющий проход; пределы поправок и число проходов
const (
	verifyTolDb  = 1.0
	verifyMaxDb  = 6.0 // за один проход
	verifySumDb  = 9.0 // сумма поправок записи за все проходы
	verifyPasses = 2   // исправляющих проходов не больше (всего сборок ≤ 3)
)

// verifiedBuild — итог: версия-трек, её файл-вариант и записи, которыми она собрана (с поправками сверки)
type verifiedBuild struct {
	child int64
	file  string
	specs []SectionSpec
}

// buildVerified — сборка итога со сверкой: render собирает файл варианта трека из записей; first — уже собранный
// теми же записями файл (сборка-прослушивание черновика): первой сборки нет. Пробная версия, после которой был
// исправляющий проход, удаляется — итог одна версия title. Сбой первой сборки — ошибка; сбой сверки или
// исправляющего прохода — остаётся последняя версия (в лог).
func buildVerified(ctx context.Context, svc yue.Service, parentID int64, specs []SectionSpec, first, title string,
	render func(context.Context, []SectionSpec) (string, error)) (*verifiedBuild, error) {
	cur := slices.Clone(specs)
	start := time.Now()
	// версия создаётся и при отмене (без отмены вызова): иначе воркер мог бы создать её, а id потерялся бы —
	// откат (OnCanceled у вызывающего, удаление пробной здесь) её бы не нашёл
	keep := context.WithoutCancel(ctx)
	file, child := first, int64(0)
	rendered := false // файл собран здесь: «сборка + сверка» — мера оставшегося времени
	if file != "" {
		id, err := svc.VariantToTrack(keep, parentID, file, title, 0)
		if err != nil {
			// файла прослушивания нет (удалён, перезаписан) — собираем заново
			log.Printf("итог #%d: сборка-прослушивание %s не подошла: %v — собираю заново", parentID, file, err)
			file = ""
		}
		child = id
	}
	if file == "" {
		ReportStep(ctx, "render", 1, nil)
		f, err := render(ctx, cur)
		if err != nil {
			return nil, err
		}
		id, err := svc.VariantToTrack(keep, parentID, f, title, 0)
		if err != nil {
			return nil, err
		}
		file, child, rendered = f, id, true
	}
	targets := verifyTargets(cur)
	if len(targets) == 0 {
		return &verifiedBuild{child: child, file: file, specs: cur}, nil
	}
	// оценка оставшегося — после первой сверки: «сборка + сверка» первого прохода минус прошедшее в текущем
	var firstPass time.Duration
	passStart := start
	eta := func() *float64 {
		if firstPass == 0 {
			return nil
		}
		v := max(0, (firstPass - time.Since(passStart)).Seconds())
		return &v
	}
	sum := map[int]float64{}
	for n := 1; n <= verifyPasses; n++ {
		ReportStep(ctx, "verify", n, eta())
		corr := verifyLevels(ctx, svc, child, cur, targets)
		if firstPass == 0 && rendered {
			firstPass = time.Since(passStart)
		}
		if len(corr) == 0 {
			break
		}
		next := slices.Clone(cur)
		for i, c := range corr {
			s := max(-verifySumDb, min(verifySumDb, sum[i]+c))
			next[i].Db += s - sum[i]
			sum[i] = s
		}
		passStart = time.Now()
		ReportStep(ctx, "fix", n+1, eta())
		f, err := render(ctx, next)
		if err != nil {
			log.Printf("итог #%d: исправляющий проход: %v — остаётся прежняя версия", parentID, err)
			break
		}
		id, err := svc.VariantToTrack(keep, parentID, f, title, 0)
		if err != nil {
			log.Printf("итог #%d: версия исправляющего прохода: %v — остаётся прежняя", parentID, err)
			break
		}
		// пробная версия только путала бы в списке — прочь (и при отмене: удаление не должно отмениться с ней)
		if _, err := svc.DeleteJob(keep, child); err != nil {
			log.Printf("итог #%d: пробная версия #%d не удалена: %v", parentID, child, err)
		}
		child, file, cur, rendered = id, f, next, true
	}
	return &verifiedBuild{child: child, file: file, specs: cur}, nil
}

// verifyTargets — цели сверки {индекс записи: дБ}: записи с VerifyDb на одну дорожку
func verifyTargets(specs []SectionSpec) map[int]float64 {
	targets := map[int]float64{}
	for i, s := range specs {
		if s.VerifyDb != nil && len(s.Stems) == 1 {
			targets[i] = *s.VerifyDb
		}
	}
	return targets
}

// verifyLevels — сверка версии: разделение RoFormer, уровни дорожек к треку (как замер эталонов); поправки
// {индекс записи: −ошибка, ±verifyMaxDb}, если хоть одна цель дальше verifyTolDb. Сбой разделения/замера — без
// поправок (в лог).
func verifyLevels(ctx context.Context, svc yue.Service, child int64, specs []SectionSpec, targets map[int]float64) map[int]float64 {
	if _, err := svc.MakeStemsWith(ctx, child, "roformer"); err != nil {
		log.Printf("сверка #%d — разделение: %v", child, err)
		return nil
	}
	got, _, err := stemLevels(ctx, svc, child)
	if err != nil {
		log.Printf("сверка #%d — замер: %v", child, err)
		return nil
	}
	// неполный замер (у основной или целевой дорожки нет rms_p95_db) — сумма неверна, промах был бы ложным
	for _, n := range presetMainStems {
		if _, ok := got[n]; !ok {
			log.Printf("сверка #%d — нет замера дорожки %s", child, n)
			return nil
		}
	}
	for i := range targets {
		if _, ok := got[specs[i].Stems[0]]; !ok {
			log.Printf("сверка #%d — нет замера дорожки %s", child, specs[i].Stems[0])
			return nil
		}
	}
	corr := map[int]float64{}
	miss := false
	for i, t := range targets {
		e := got[specs[i].Stems[0]] - t
		corr[i] = max(-verifyMaxDb, min(verifyMaxDb, -e))
		if math.Abs(e) > verifyTolDb {
			miss = true
		}
	}
	if !miss {
		return nil
	}
	log.Printf("сверка #%d — поправки %v", child, corr)
	return corr
}
