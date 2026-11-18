package waveform_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// markChart is a chart with a known sine and app-owned anchors, so a mode
// switch that rebuilds the view tree cannot lose the markers' positions.
func markChart(cx *core.Scope, mode waveform.MarkReading, markA, markB *core.Signal[float64], readings ...waveform.Measurement) core.Node {
	y := make([]float32, 1000)
	for i := range y {
		y[i] = float32(100 * math.Sin(2*math.Pi*float64(i)/100))
	}
	return waveform.Chart{
		TestID: "wave",
		Series: []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Color: core.RGB(0x1a7f37)}},
		Measure: true, MarkLabels: [2]string{"A", "B"},
		MarkReading: mode, MarkA: markA, MarkB: markB,
		Measurements: readings,
		Style:        core.Style{Width: core.Pt(600), Height: core.Pt(300)},
	}
}

func TestValueModeDrawsHorizontalLinesAndReadsDelta(t *testing.T) {
	var markA, markB *core.Signal[float64]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		// 0.125 and 0.375 land on a peak and a trough of the sine, so the
		// two horizontal lines must sit at different heights.
		markA, markB = core.NewSignal(cx, 0.125), core.NewSignal(cx, 0.375)
		return markChart(cx, waveform.MarkReadingValue, markA, markB,
			waveform.Measurement{Source: 0, Kind: waveform.MeasureMarkDelta, Label: "Δ="})
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !app.Exists("text:Δ=") {
		t.Fatalf("the Δ= reading did not show; tree:\n%s", app.Dump())
	}
	// Horizontal lines have a meaningful y position and share the full
	// plot width; the anchors are distinct values on the wave.
	a, b := app.Find("id:wave-mark-a").Frame, app.Find("id:wave-mark-b").Frame
	if a.Origin.Y == b.Origin.Y {
		t.Fatalf("both horizontal markers sit at the same height: %v", a.Origin.Y)
	}
	if a.Size.Width <= 0 || a.Size.Height > 2 {
		t.Fatalf("a horizontal marker is not a full-width thin line: %+v", a.Size)
	}
}

func TestTimeModeDrawsVerticalLines(t *testing.T) {
	var markA, markB *core.Signal[float64]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		markA, markB = core.NewSignal(cx, 0.25), core.NewSignal(cx, 0.75)
		return markChart(cx, waveform.MarkReadingTime, markA, markB,
			waveform.Measurement{Source: 0, Kind: waveform.MeasureSpan, Label: "Δt", Over: waveform.MeasureMarkers})
	}, nsgotest.Size(800, 600))
	app.Settle()
	a, b := app.Find("id:wave-mark-a").Frame, app.Find("id:wave-mark-b").Frame
	if a.Origin.X == b.Origin.X {
		t.Fatalf("both vertical markers sit at the same x: %v", a.Origin.X)
	}
	if a.Size.Height <= 0 || a.Size.Width > 2 {
		t.Fatalf("a vertical marker is not a full-height thin line: %+v", a.Size)
	}
}

func TestNoneHidesMarkers(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		markA, markB := core.NewSignal(cx, 0.25), core.NewSignal(cx, 0.75)
		return markChart(cx, waveform.MarkReadingNone, markA, markB)
	}, nsgotest.Size(800, 600))
	app.Settle()
	if app.Exists("id:wave-mark-a") || app.Exists("id:wave-mark-b") {
		t.Fatal("markers still drawn while hidden")
	}
}

func TestSwitchingOrientationKeepsAnchors(t *testing.T) {
	var mode *core.Signal[waveform.MarkReading]
	var markA, markB *core.Signal[float64]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		mode = core.NewSignal(cx, waveform.MarkReadingTime)
		markA, markB = core.NewSignal(cx, 0.3), core.NewSignal(cx, 0.7)
		return core.Dynamic(func(cx *core.Scope) core.Node {
			return markChart(cx, mode.Get(), markA, markB)
		})
	}, nsgotest.Size(800, 600))
	app.Settle()
	xBefore := app.Find("id:wave-mark-a").Frame.Origin.X
	app.Post(func() { mode.Set(waveform.MarkReadingValue) })
	app.Settle()
	yAfter := app.Find("id:wave-mark-a").Frame.Origin.Y
	if math.IsNaN(markA.Get()) || math.Abs(markA.Get()-0.3) > 1e-9 {
		t.Fatalf("the anchor moved during the switch: %v", markA.Get())
	}
	if yAfter <= 0 {
		t.Fatalf("the horizontal marker has no height after switching: %v", yAfter)
	}
	if xBefore <= 0 {
		t.Fatalf("the vertical marker had no x before switching: %v", xBefore)
	}
}

func TestValueDragMovesTheHorizontalMarker(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		markA, markB := core.NewSignal(cx, 0.125), core.NewSignal(cx, 0.375)
		return markChart(cx, waveform.MarkReadingValue, markA, markB)
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := app.Find("id:wave-mark-a").Frame.Origin
	// Start the drag on marker A's own line, then move vertically to a
	// different value on the wave.
	app.DragAt("id:wave",
		driver.Point{X: before.X + 40, Y: before.Y},
		driver.Point{X: 0, Y: 40})
	app.Settle()
	after := app.Find("id:wave-mark-a").Frame.Origin.Y
	if after == before.Y {
		t.Fatalf("dragging did not move the horizontal marker: %v", before.Y)
	}
}
