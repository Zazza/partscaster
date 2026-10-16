package dsp

import (
	"encoding/json"
	"sort"
	"testing"
)

// Группы цепочек для общего списка эффектов (internal-voice-fx, ТК1):
// у каждой цепочки группа из 8 разделов docs/{ru,en}/effects.md, группа
// voice ⇔ Voice, ForVoice — ровно общие эффекты, уместные на голосе.

var chainGroups = map[string]bool{
	"space": true, "pedals": true, "modulation": true, "filter": true,
	"dynamics": true, "time": true, "lofi": true, "voice": true,
}

var wantForVoice = []string{"soften", "reverb-room", "reverb-hall", "reverb-plate", "haas", "pitch", "octaver"}

func TestChainsHaveKnownGroup(t *testing.T) {
	t.Parallel()
	all := All()
	if len(all) == 0 {
		t.Fatal("All() пуст")
	}
	for _, c := range all {
		if !chainGroups[c.Group] {
			t.Errorf("%s: Group=%q, want одну из space/pedals/modulation/filter/dynamics/time/lofi/voice", c.ID, c.Group)
		}
	}
}

func TestVoiceGroupIffVoice(t *testing.T) {
	t.Parallel()
	for _, c := range All() {
		if (c.Group == "voice") != c.Voice {
			t.Errorf("%s: Group=%q, Voice=%v — группа voice должна быть ровно у голосовых", c.ID, c.Group, c.Voice)
		}
	}
	// голосовые цепочки из карточки — именно в группе voice
	for _, id := range []string{"megaphone", "phone", "voice-drive", "slapback"} {
		c := ByID(id)
		if c == nil {
			t.Fatalf("ByID(%q) = nil", id)
		}
		if c.Group != "voice" {
			t.Errorf("%s: Group=%q, want voice", id, c.Group)
		}
	}
}

func TestForVoiceExactList(t *testing.T) {
	t.Parallel()
	var got []string
	for _, c := range All() {
		if c.ForVoice {
			got = append(got, c.ID)
			if c.Group == "voice" {
				t.Errorf("%s: ForVoice у цепочки группы voice — общий эффект не может быть голосовым", c.ID)
			}
			if c.Voice {
				t.Errorf("%s: ForVoice и Voice одновременно", c.ID)
			}
		}
	}
	want := append([]string(nil), wantForVoice...)
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("ForVoice = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ForVoice = %v, want %v", got, want)
		}
	}
}

// Поля уходят в JSON (биндинг и MCP dsp_chains): group всегда, for_voice — только у true.
func TestChainGroupJSON(t *testing.T) {
	t.Parallel()
	enc := func(c *Chain) map[string]any {
		t.Helper()
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("%s: marshal: %v", c.ID, err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	rev := ByID("reverb-hall")
	if rev == nil {
		t.Fatal("ByID(reverb-hall) = nil")
	}
	m := enc(rev)
	if m["group"] != rev.Group || rev.Group == "" {
		t.Errorf("reverb-hall: json group=%v, Group=%q", m["group"], rev.Group)
	}
	if m["for_voice"] != true {
		t.Errorf("reverb-hall: json for_voice=%v, want true", m["for_voice"])
	}
	for _, c := range All() {
		if c.ForVoice {
			continue
		}
		cc := c
		m := enc(&cc)
		if _, ok := m["for_voice"]; ok {
			t.Errorf("%s: for_voice в JSON у цепочки без ForVoice (нужен omitempty)", c.ID)
		}
		if _, ok := m["group"]; !ok {
			t.Errorf("%s: нет поля group в JSON", c.ID)
		}
	}
}
