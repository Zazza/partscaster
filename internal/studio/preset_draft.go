package studio

import (
	"context"
	"log"
	"maps"
	"slices"

	"partscaster/internal/dsp"
	"partscaster/internal/yue"
)

// PresetDraftResult — пресет, готовый лечь в черновик студии одной записью: правки пересборки с посчитанными
// громкостями (и поправками сверки баланса) и партиями, финал на весь микс, мастер с целевой громкостью.
// «Применить всё» черновика собирает из них трек вместе с правками человека.
type PresetDraftResult struct {
	Sections []SectionSpec    `json:"sections"`
	Final    []dsp.Step       `json:"final,omitempty"`
	Master   []map[string]any `json:"master,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

// PresetDraft — пресет в черновик: та же подготовка и сверка баланса, что у ApplySoundPreset, но трек не
// меняется — версии сверки временные (удаляются к концу при любом исходе), итоговой версии и записи журнала нет.
// Нечего сверять (нет целей level_db с замером) — сборки нет вовсе. Одна операция общей очереди (вид preset).
func PresetDraft(ctx context.Context, svc yue.Service, jobID int64, p yue.SoundPreset) (*PresetDraftResult, error) {
	var res *PresetDraftResult
	_, err := RunOp(ctx, svc, "preset", p.Name, jobID, func(ctx context.Context) (int64, error) {
		r, err := presetDraft(yue.WithoutJournal(ctx), svc, jobID, p)
		if err != nil {
			return 0, err
		}
		res = r
		return 0, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func presetDraft(ctx context.Context, svc yue.Service, jobID int64, p yue.SoundPreset) (*PresetDraftResult, error) {
	pr, err := preparePreset(ctx, svc, jobID, p)
	if err != nil {
		return nil, err
	}
	used, err := presetDraftVerify(ctx, svc, pr)
	if err != nil {
		return nil, err
	}
	out := &PresetDraftResult{Sections: presetSpecs(pr.p, pr.parts, pr.levels, pr.total, used),
		Master: presetMaster(pr.p), Warnings: pr.warnings}
	if len(out.Master) == 0 {
		out.Master = nil
	}
	out.Final = activeSteps(pr.p.Final)
	return out, nil
}

// presetDraftVerify — сверка баланса по временным версиям (как у ApplySoundPreset): ответ — поправки записей
// последней удачной сборки. Первая сборка не удалась — ошибка; исправляющий проход не удался — остаются прежние.
func presetDraftVerify(ctx context.Context, svc yue.Service, pr *presetPrep) (map[int]float64, error) {
	if len(presetVerifyTargets(pr.p.Specs, pr.levels)) == 0 {
		return nil, nil
	}
	var temps []int64
	var files []string
	made := func(f string) {
		if !slices.Contains(files, f) {
			files = append(files, f)
		}
	}
	defer func() {
		// временные версии и пробные варианты у трека — прочь при любом исходе: отмена операции ctx уже отменила.
		// Вариант звучит без правок человека — в «результатах» он только путал бы
		c := context.WithoutCancel(ctx)
		for _, id := range temps {
			if _, err := svc.DeleteJob(c, id); err != nil {
				log.Printf("пресет «%s» в черновик: временная версия #%d не удалена: %v", pr.p.Name, id, err)
			}
		}
		for _, f := range files {
			if _, err := svc.DspVariantDelete(c, pr.job.ID, f); err != nil {
				log.Printf("пресет «%s» в черновик: пробный вариант %s не удалён: %v", pr.p.Name, f, err)
			}
		}
	}()
	name := pr.title() + " · " + pr.p.Name + " · проверка баланса (временная)"
	var used map[int]float64
	next := map[int]float64{}
	for pass := 0; pass <= presetVerifyPasses; pass++ {
		if pass == 0 {
			ReportStep(ctx, "render", 1, nil)
		} else {
			ReportStep(ctx, "fix", pass+1, nil)
		}
		file, err := renderPreset(ctx, svc, pr.job, pr.p, pr.parts, pr.levels, pr.total, next, made)
		if err == nil {
			var child int64
			if child, err = svc.VariantToTrack(ctx, pr.job.ID, file, name, 0); err == nil {
				temps = append(temps, child)
				used = maps.Clone(next)
				if pass == presetVerifyPasses {
					break
				}
				ReportStep(ctx, "verify", pass+1, nil)
				corr := presetVerify(ctx, svc, child, pr.p.Specs, pr.levels)
				if len(corr) == 0 {
					break
				}
				for i, c := range corr {
					next[i] = max(-presetVerifySumDb, min(presetVerifySumDb, next[i]+c))
				}
				continue
			}
		}
		if pass == 0 {
			return nil, err
		}
		log.Printf("пресет «%s» в черновик: исправляющий проход: %v — остаются прежние поправки", pr.p.Name, err)
		break
	}
	return used, nil
}
