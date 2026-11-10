package waveform_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestChartDragPansTheView(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	var views []waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:       "wave",
			Series:       []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			OnViewChange: func(v waveform.Viewport) { views = append(views, v) },
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := len(views)
	app.DragAt("id:wave", driver.Point{X: 300, Y: 150}, driver.Point{X: 80, Y: 0})
	app.Settle()
	if len(views) <= before {
		t.Fatalf("a drag did not pan: %v", views)
	}
	if v := views[len(views)-1]; !v.X.Valid() || v.X.Lo == 0 {
		t.Fatalf("the drag's view is %v, want a panned range", v.X)
	}
}

// A drag's steps must move the view one way only. Reading each step's
// pointer in the view the drag is producing made every step's pan steer
// the next step's reading, and the waveform shook in place instead of
// sliding.
func TestDragPansOneWayOnly(t *testing.T) {
	y := make([]float32, 10000)
	for i := range y {
		y[i] = float32(i % 100)
	}
	var views []waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:       "wave",
			Series:       []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Color: core.RGB(0x288cf0)}},
			View:         waveform.Viewport{X: waveform.Range{Lo: 2, Hi: 4}},
			OnViewChange: func(v waveform.Viewport) { views = append(views, v) },
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	views = nil
	app.DragAt("id:wave", driver.Point{X: 400, Y: 150}, driver.Point{X: 120, Y: 0})
	app.Settle()
	if len(views) < 3 {
		t.Fatalf("the drag reported %d views", len(views))
	}
	sign := 0.0
	for i := 1; i < len(views); i++ {
		d := views[i].X.Lo - views[i-1].X.Lo
		if d == 0 {
			continue
		}
		if sign == 0 {
			sign = d
			continue
		}
		if d > 0 != (sign > 0) {
			t.Fatalf("step %d went back: %v then %v", i, views[i-1].X, views[i].X)
		}
	}
	if last := views[len(views)-1]; last.X.Lo == 2 || last.X.Lo == views[0].X.Lo {
		t.Fatalf("the drag panned to %v", last.X)
	}
}
