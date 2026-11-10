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

// The TSX props must carry every new field through to the base chart,
// including the interval's source and the trigger's mode.
func TestBaseChartMapsScopeProps(t *testing.T) {
	c := Chart{
		Snap:       func() bool { return true },
		XLog:       func() bool { return true },
		Data:       func() [][]float32 { return [][]float32{{1, 2, 3}} },
		MarkLabels: func() []string { return []string{"A", "B"} },
		Channels: func() []Channel {
			return []Channel{{Offset: 2, Invert: true, AC: true}}
		},
		Trigger: func() *Trigger { return &Trigger{Mode: "single", Edge: "falling"} },
		Measurements: func() []Measurement {
			return []Measurement{
				{Source: 0, Kind: "span", Label: "Δt", Over: "markers"},
				{Source: 0, Kind: "rate", Label: "1/Δt"},
			}
		},
	}
	b := c.BaseChart()
	if !b.Snap || !b.XLog {
		t.Fatalf("scope props lost: %+v", b)
	}
	if b.Trigger == nil || b.Trigger.Mode != base.TriggerSingle || b.Trigger.Edge != base.TriggerFalling {
		t.Fatalf("trigger: %+v", b.Trigger)
	}
	if len(b.Measurements) != 2 ||
		b.Measurements[0].Kind != base.MeasureSpan || b.Measurements[0].Over != base.MeasureMarkers ||
		b.Measurements[1].Kind != base.MeasureRate || b.Measurements[1].Over != base.MeasureView {
		t.Fatalf("measurements: %+v", b.Measurements)
	}
	if ch := b.Series[0]; ch.Offset != 2 || !ch.Invert || !ch.AC {
		t.Fatalf("channel: %+v", ch)
	}
	if b.MarkLabels != [2]string{"A", "B"} {
		t.Fatalf("marker labels: %+v", b.MarkLabels)
	}
}

// Every kind the type declarations name must map to its own base kind,
// rather than silently falling back to Vpp as "span" and "rate" once did.
func TestEveryMeasurementKindMaps(t *testing.T) {
	cases := map[string]base.MeasureKind{
		"vpp": base.MeasureVpp, "max": base.MeasureMax, "min": base.MeasureMin,
		"mean": base.MeasureMean, "rms": base.MeasureRMS,
		"frequency": base.MeasureFrequency, "period": base.MeasurePeriod,
		"duty": base.MeasureDuty, "rise": base.MeasureRise,
		"fall": base.MeasureFall, "span": base.MeasureSpan, "rate": base.MeasureRate,
	}
	for name, want := range cases {
		if got := kindOf(name); got != want {
			t.Errorf("kindOf(%q) = %v, want %v", name, got, want)
		}
	}
}
