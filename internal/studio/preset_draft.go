package studio

import (
	"context"

	"partscaster/internal/dsp"
	"partscaster/internal/yue"
)

// PresetDraftResult — пресет, готовый лечь в черновик студии одной записью: правки пересборки с посчитанными
// громкостями (и целями сверки баланса) и партиями, финал на весь микс, мастер с целевой громкостью.
// «Применить всё» черновика собирает из них трек вместе с правками человека.
type PresetDraftResult struct {
	Sections []SectionSpec    `json:"sections"`
	Final    []dsp.Step       `json:"final,omitempty"`
	Master   []map[string]any `json:"master,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

// PresetDraft — пресет в черновик: та же подготовка, что у ApplySoundPreset, без сборок — трек не меняется, версий
// нет. Сверка баланса — при «применить всё» черновика (ApplyEdits: записи несут цели VerifyDb).
// Одна операция общей очереди (вид preset).
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
	out := &PresetDraftResult{Sections: presetSpecs(pr.p, pr.parts, pr.levels, pr.total),
		Master: presetMaster(pr.p), Warnings: pr.warnings}
	if len(out.Master) == 0 {
		out.Master = nil
	}
	out.Final = activeSteps(pr.p.Final)
	return out, nil
}
