package waveform_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A desktop backend reports a mouse press as a finger too, for the views
// that only know touches. A chart knows both, so the touches beside its
// mouse drag must not steer the view: the drag has to pan as a mouse does,
// to the end the pointer reached.
func TestDragPansWhenTheMouseIsAlsoAFinger(t *testing.T) {
	y := make([]float32, 1000)
	for i := range y {
		y[i] = float32(i % 10)
	}
	var last waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:       "wave",
			Series:       []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Color: core.RGB(0x288cf0)}},
			View:         waveform.Viewport{X: waveform.Range{Lo: 0.2, Hi: 0.4}},
			OnViewChange: func(v waveform.Viewport) { last = v },
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	const start = 0.2
	f := app.Touch("id:wave", driver.Point{X: 300, Y: 150})
	f.Move(driver.Point{X: 240, Y: 150})
	f.Move(driver.Point{X: 180, Y: 150})
	f.Lift()
	app.Settle()
	if !last.X.Valid() || last.X.Lo <= start+0.01 {
		t.Fatalf("the drag ended at %v, want it panned left, past %v", last.X, start+0.01)
	}
}
