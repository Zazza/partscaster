package studio

import (
	"context"
	"fmt"

	"yue-studio/internal/dsp"
	"yue-studio/internal/yue"
)

// pedalsFile — вариант «педали на весь трек»
const pedalsFile = "dsp-pedals.flac"

// ApplyChain — DSP-цепочка ffmpeg на весь трек: вариант dsp-<цепочка>.flac (общий конвейер приложения и MCP).
// В журнал правок — запись dsp_apply.
func ApplyChain(ctx context.Context, svc yue.Service, jobID int64, chainID string, params map[string]float64) (*yue.DspVariant, error) {
	chain := dsp.ByID(chainID)
	if chain == nil {
		return nil, fmt.Errorf("unknown chain %q (см. dsp_chains)", chainID)
	}
	if chain.Key != "" {
		return nil, fmt.Errorf("эффект %q — только на дорожку (ключ — дорожка %s)", chain.Name, chain.Key)
	}
	v, err := RunGraph(yue.WithoutJournal(ctx), svc, jobID, "", chain.FilterGraph(params), fmt.Sprintf("dsp-%s.flac", chainID), "")
	if err != nil {
		return nil, err
	}
	yue.Record(ctx, svc, jobID, yue.JobOp{Op: "dsp_apply", Params: yue.Params(map[string]any{"chain": chainID, "params": params}),
		Result: v.File})
	return v, nil
}

// ApplySteps — цепочка эффектов по порядку (доска педалей) на весь трек: вариант dsp-pedals.flac с подписью
// label ("" — названия педалей). В журнал правок — запись pedals.
func ApplySteps(ctx context.Context, svc yue.Service, jobID int64, steps []dsp.Step, label string) (*yue.DspVariant, error) {
	graph, _, err := dsp.StepsGraph(steps)
	if err != nil {
		return nil, err
	}
	if label == "" {
		label = StepsLabel(steps)
	}
	v, err := RunGraph(yue.WithoutJournal(ctx), svc, jobID, "", graph, pedalsFile, label)
	if err != nil {
		return nil, err
	}
	yue.Record(ctx, svc, jobID, yue.JobOp{Op: "pedals", Params: yue.Params(map[string]any{"steps": steps, "label": label}),
		Result: v.File})
	return v, nil
}
