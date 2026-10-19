package yue

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) References(ctx context.Context) ([]Reference, error) {
	var refs []Reference
	if err := c.get(ctx, "/references", &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

// AddReference — залить референс на воркер (там замер метрик).
func (c *Client) AddReference(ctx context.Context, name string, data []byte) (*Reference, error) {
	var out Reference
	if err := c.postRaw(ctx, "/references", name, data, planTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) JobDspVariants(ctx context.Context, id int64) ([]DspVariant, error) {
	var vs []DspVariant
	if err := c.get(ctx, fmt.Sprintf("/jobs/%d/dsp", id), &vs); err != nil {
		return nil, err
	}
	return vs, nil
}

// UploadDsp — залить DSP-вариант джобы (включает librosa-замер на воркере).
func (c *Client) UploadDsp(ctx context.Context, id int64, fname, label string, data []byte) (*DspVariant, error) {
	path := fmt.Sprintf("/jobs/%d/dsp", id)
	if label != "" {
		path += "?label=" + url.QueryEscape(label)
	}
	var out DspVariant
	if err := c.postRaw(ctx, path, fname, data, planTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Transcribe — трек → ABC (SheetSage2; первый вызов грузит модель).
func (c *Client) Transcribe(ctx context.Context, name string, data []byte) (*TranscribeResult, error) {
	var out TranscribeResult
	if err := c.postRaw(ctx, "/transcribe", name, data, gpuTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MakeStems — дорожки (RoFormer): drums/bass/other/vocals + подробные guitar/piano и части барабанов
// kick/snare/toms/hh/ride/crash; model — bs-roformer-sw.
func (c *Client) MakeStems(ctx context.Context, id int64) (map[string]any, error) {
	var out map[string]any
	if err := c.post(ctx, fmt.Sprintf("/jobs/%d/stems", id), gpuTimeout, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuitarPart — что сыграет гитарист «по аккордам плана» (блок движка gtrnotes): такты с аккордами и частями песни,
// удары с нотами в манере style (0 авто, 1 бой, 2 перебор, 3 бас + аккорд, 4 квинты). Первый расчёт трека — секунды
// без видеокарты (удары, доли, строй), дальше — кэш у трека. Ответ {style, bars, strikes, beats, tune_cents}.
func (c *Client) GuitarPart(ctx context.Context, id int64, style int) (map[string]any, error) {
	var out map[string]any
	path := fmt.Sprintf("/jobs/%d/guitar_part?style=%d", id, style)
	if err := c.call(ctx, http.MethodGet, path, planTimeout, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DspVariantDelete — удалить вариант эффекта/вклейки (файл + метрики).
func (c *Client) DspVariantDelete(ctx context.Context, id int64, fname string) (bool, error) {
	var out struct {
		Deleted bool `json:"deleted"`
	}
	if err := c.del(ctx, fmt.Sprintf("/jobs/%d/dsp/%s", id, fname), &out); err != nil {
		return false, err
	}
	return out.Deleted, nil
}

func (c *Client) JobStems(ctx context.Context, id int64) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.get(ctx, fmt.Sprintf("/jobs/%d/stems", id), &out); err != nil {
		return nil, err
	}
	return out, nil
}
