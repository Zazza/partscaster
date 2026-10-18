package studio

import (
	"context"
	"fmt"
	"slices"

	"partscaster/internal/yue"
)

// PresetRef — пресет звука в черновике студии одной записью: рецепт целиком (с громкостями «на этот раз»), без
// подготовки. Сборка (пересборка для прослушивания, «применить всё») раскрывает его подготовкой — дорожки, план,
// партии, слой, наборы, замер (preparePreset) — в записи пересборки. Правки человека перекрывают правки пресета:
// звук дорожек SkipSound, место дорожек SkipPlace, мастер (SkipMaster) — их записи пресета не идут.
type PresetRef struct {
	Preset     yue.SoundPreset `json:"preset"`
	SkipSound  []string        `json:"skip_sound,omitempty"`
	SkipPlace  []string        `json:"skip_place,omitempty"`
	SkipMaster bool            `json:"skip_master,omitempty"`
}

// expandPresets — записи пересборки с раскрытыми пресетами (на своём месте, с ID записи-пресета) и предупреждения
// подготовки (у трека нет плана — гитарист пропущен). Записей-пресетов нет — те же записи.
func expandPresets(ctx context.Context, svc yue.Service, parentID int64, specs []SectionSpec) ([]SectionSpec, []string, error) {
	if !slices.ContainsFunc(specs, func(s SectionSpec) bool { return s.Preset != nil }) {
		return specs, nil, nil
	}
	out := make([]SectionSpec, 0, len(specs))
	var warnings []string
	for _, s := range specs {
		if s.Preset == nil {
			out = append(out, s)
			continue
		}
		ref := s.Preset
		pr, err := preparePreset(ctx, svc, parentID, ref.Preset)
		if err != nil {
			return nil, nil, &EditError{ID: s.ID, Err: fmt.Errorf("пресет «%s»: %w", ref.Preset.Name, err)}
		}
		for _, w := range pr.warnings {
			warnings = append(warnings, ref.Preset.Name+": "+w)
		}
		for _, sec := range presetSpecs(pr.p, pr.parts, pr.levels, pr.total) {
			sound := !sec.Add && (sec.Chain != "" || len(sec.Steps) > 0 || len(sec.Engine) > 0)
			if sound && slices.ContainsFunc(sec.Stems, func(n string) bool { return slices.Contains(ref.SkipSound, n) }) {
				continue
			}
			if sec.isPlace() && len(sec.Stems) == 1 && slices.Contains(ref.SkipPlace, sec.Stems[0]) {
				continue
			}
			sec.ID = s.ID
			out = append(out, sec)
		}
		if steps := activeSteps(pr.p.Final); len(steps) > 0 {
			out = append(out, SectionSpec{ID: s.ID, Stems: []string{"mix"}, Mix: true, Steps: steps})
		}
		if m := presetMaster(pr.p); len(m) > 0 && !ref.SkipMaster {
			out = append(out, SectionSpec{ID: s.ID, Master: true, Engine: m})
		}
	}
	return out, warnings, nil
}

// EditError — сбой одной правки черновика при сборке: ID — её запись (студия красит строку). Текст несёт отметку
// {edit:ID} — ошибка до интерфейса доходит строкой.
type EditError struct {
	ID  string
	Err error
}

func (e *EditError) Error() string {
	if e.ID == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + " {edit:" + e.ID + "}"
}

func (e *EditError) Unwrap() error { return e.Err }

// editErr — ошибка правки s с её ID (без ID — как есть)
func editErr(s SectionSpec, err error) error {
	if err == nil || s.ID == "" {
		return err
	}
	return &EditError{ID: s.ID, Err: err}
}
