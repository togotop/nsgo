package waveform_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestChartShowsAutomaticReadings(t *testing.T) {
	y := make([]float32, 1000)
	for i := range y {
		y[i] = float32(100 * math.Sin(2*math.Pi*float64(i)/100))
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Color: core.RGB(0x1a7f37)}},
			Measurements: []waveform.Measurement{
				{Source: 0, Kind: waveform.MeasureVpp, Label: "Vpp"},
				{Source: 0, Kind: waveform.MeasureFrequency, Label: "f"},
			},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	for _, id := range []string{"id:wave-readings", "id:wave-m0", "id:wave-m1"} {
		if !app.Exists(id) {
			t.Errorf("no reading view %s", id)
		}
	}
	if !app.Exists("text:Vpp") {
		t.Fatalf("the reading's label did not show; tree:\n%s", app.Dump())
	}
}

func TestChartReadsBetweenTheMarkers(t *testing.T) {
	y := make([]float32, 1000)
	for i := range y {
		y[i] = float32(100 * math.Sin(2*math.Pi*float64(i)/100))
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:  "wave",
			Series:  []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Color: core.RGB(0x1a7f37)}},
			Measure: true,
			Measurements: []waveform.Measurement{
				{Source: 0, Kind: waveform.MeasureSpan, Label: "Δt", Over: waveform.MeasureMarkers},
				{Source: 0, Kind: waveform.MeasureVpp, Label: "Vpp", Over: waveform.MeasureMarkers},
			},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	for _, want := range []string{"text:Δt", "text:Vpp"} {
		if !app.Exists(want) {
			t.Fatalf("no reading %q; tree:\n%s", want, app.Dump())
		}
	}
}
