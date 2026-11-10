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
