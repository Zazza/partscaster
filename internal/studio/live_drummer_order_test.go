package studio

import (
	"testing"

	"partscaster/internal/yue"
)

// Условие 2 (решение по неясности): сбивка в последний припев (2 такта) длиннее прежней части в 1 такт — точки
// линии громкости барабанов всё равно идут по неубыванию времени (иначе блок level отклонит цепочку).
func TestLiveDrumPoints_FillLongerThanPrevPart_Ordered(t *testing.T) {
	bar := 2.0
	secs := []string{"verse", "verse", "chorus", "chorus", "bridge", "chorus", "chorus"}
	bars := make([]yue.ChordBar, len(secs))
	for i, s := range secs {
		bars[i] = yue.ChordBar{Start: float64(i) * bar, End: float64(i+1) * bar, Chord: "C", Section: s}
	}
	pts := liveDrumPoints(bars, 0)
	if len(pts) == 0 {
		t.Fatal("нет точек линии")
	}
	prev := -1.0
	for k, p := range pts {
		tt, _ := p["t"].(float64)
		if tt < prev {
			t.Fatalf("точка %d: t=%.3f раньше прежней %.3f — линия не по порядку: %v", k, tt, prev, pts)
		}
		prev = tt
	}
}

// Ревью, круг 1: такты за концом плана (часть "") держат громкость последней части — припев в конце плана не
// обрывается скачком −4,5 дБ.
func TestLiveDrumPoints_PastPlanHoldsLastSection(t *testing.T) {
	secs := []string{"verse", "chorus", "chorus", "", ""}
	bars := make([]yue.ChordBar, len(secs))
	for i, s := range secs {
		bars[i] = yue.ChordBar{Start: float64(i) * 2, End: float64(i+1) * 2, Chord: "C", Section: s}
	}
	pts := liveDrumPoints(bars, 0)
	last, _ := pts[len(pts)-1]["db"].(float64)
	if last != liveDrumChorusDb {
		t.Fatalf("за концом плана громкость %.1f дБ, ждали как у припева %.1f: %v", last, liveDrumChorusDb, pts)
	}
}
