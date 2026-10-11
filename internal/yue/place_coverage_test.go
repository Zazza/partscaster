package yue

import (
	"encoding/json"
	"testing"
)

// Спецификация Place: ширина не задана — 1 («как есть»), а не 0 (моно):
// {"pan": 0.3} не должен сводить дорожку в моно. Явная ширина (в т.ч. 0) — как задана.

func TestPlaceWidthDefaultsToOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in         string
		pan, width float64
	}{
		{`{"pan":0.3}`, 0.3, 1},
		{`{}`, 0, 1},
		{`{"pan":-1}`, -1, 1},
		{`{"pan":0.3,"width":0}`, 0.3, 0},
		{`{"width":1.8}`, 0, 1.8},
		{`{"pan":0.5,"width":0.4}`, 0.5, 0.4},
	} {
		var p Place
		if err := json.Unmarshal([]byte(tc.in), &p); err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if p.Pan != tc.pan || p.Width != tc.width {
			t.Errorf("%s → pan %v width %v, want %v %v", tc.in, p.Pan, p.Width, tc.pan, tc.width)
		}
	}
}

func TestPlaceWidthDefaultInsidePresets(t *testing.T) {
	t.Parallel()
	var sp SoundPreset
	in := `{"name":"x","specs":[{"place":{"pan":-0.4}}],"parts":[{"kind":"synth","engine":[],"place":{"pan":0.2}}]}`
	if err := json.Unmarshal([]byte(in), &sp); err != nil {
		t.Fatal(err)
	}
	if len(sp.Specs) != 1 || sp.Specs[0].Place == nil || sp.Specs[0].Place.Width != 1 {
		t.Fatalf("spec place: %+v", sp.Specs)
	}
	if len(sp.Parts) != 1 || sp.Parts[0].Place == nil || sp.Parts[0].Place.Width != 1 {
		t.Fatalf("part place: %+v", sp.Parts)
	}
	if sp.Specs[0].Place.Pan != -0.4 || sp.Parts[0].Place.Pan != 0.2 {
		t.Fatalf("pan потерян: %+v %+v", sp.Specs[0].Place, sp.Parts[0].Place)
	}
}

func TestPlaceRoundTripKeepsWidth(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(Place{Pan: 0.3, Width: 0})
	if err != nil {
		t.Fatal(err)
	}
	var p Place
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Width != 0 || p.Pan != 0.3 {
		t.Fatalf("моно после круга сериализации потерялось: %+v (%s)", p, b)
	}
}

func TestPlaceBadJSONIsError(t *testing.T) {
	t.Parallel()
	for _, in := range []string{`{"pan":"left"}`, `[1,2]`, `{"width":`} {
		var p Place
		if err := json.Unmarshal([]byte(in), &p); err == nil {
			t.Errorf("%s: ошибки нет, %+v", in, p)
		}
	}
}
