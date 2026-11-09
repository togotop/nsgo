package waveform

import (
	"testing"

	base "github.com/nsgo-dev/nsgo/base/waveform"
)

func TestChartBuildsTheBaseChart(t *testing.T) {
	c := Chart{
		Data:   func() [][]float32 { return [][]float32{{1, 2, 3}, {4, 5, 6}} },
		Names:  func() []string { return []string{"CH1"} },
		Height: func() float64 { return 240 },
		Cursor: func() bool { return true },
	}
	b := c.BaseChart()
	if len(b.Series) != 2 || b.Series[0].Name != "CH1" || b.Series[1].Name != "" {
		t.Fatalf("series: %+v", b.Series)
	}
	if b.Height != 240 || !b.Cursor {
		t.Fatalf("props: %+v", b)
	}
	if _, ok := c.Render(nil).(base.Chart); !ok {
		t.Fatalf("Chart rendered %T, want base.Chart", c.Render(nil))
	}
}

func TestNilAccessorsAreSafe(t *testing.T) {
	var c Chart
	if b := c.BaseChart(); len(b.Series) != 0 || b.Height != 0 {
		t.Fatalf("zero props made a chart: %+v", b)
	}
}
