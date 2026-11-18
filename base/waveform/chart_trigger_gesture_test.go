package waveform_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// periodic is a strict 50-sample cycle.
func periodic(n int) []float32 {
	y := make([]float32, n)
	for i := range y {
		y[i] = float32(100 * math.Sin(2*math.Pi*float64(i)/50))
	}
	return y
}

func lockedChart(src *waveform.Source, run *waveform.Run, onView func(waveform.Viewport)) core.Node {
	return waveform.Chart{
		TestID: "wave", Series: []waveform.Series{{Source: src}},
		Trigger: &waveform.Trigger{Source: 0, Level: 0, Edge: waveform.TriggerRising, Position: 0.5},
		Run:     run, Follow: 1, OnViewChange: onView,
		Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
	}
}

// The grid canvas draws a vertical line in the trigger's own color
// (RGBA(0xcc3333, 0.8) = 3425907660) at the locked edge. A locked trigger
// re-anchors on each new edge, so the line is still there after fresh
// samples arrive; a chart that slipped into follow mode keeps an old edge
// that slides out of the view, and the line disappears.
func triggerLocked(app *nsgotest.App, src *waveform.Source) bool {
	// Enough samples for a stale edge to slide out of a following view:
	// one push is not always enough to tell the two states apart.
	for i := 0; i < 2; i++ {
		app.Post(func() { src.Push(periodic(500)) })
		app.Settle()
	}
	return strings.Contains(fmt.Sprint(app.Find("id:wave-grid").Ops), "3425907660")
}

func TestTriggerSurvivesZoom(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(periodic(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return lockedChart(src, &waveform.Run{}, nil)
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !triggerLocked(app, src) {
		t.Fatal("the chart was not trigger-locked to begin with")
	}
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if !triggerLocked(app, src) {
		t.Fatal("a zoom took the trigger away")
	}
}

func TestTriggerReturnsAfterPan(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(periodic(10000))
	var views []waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return lockedChart(src, &waveform.Run{}, func(v waveform.Viewport) { views = append(views, v) })
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !triggerLocked(app, src) {
		t.Fatal("the chart was not trigger-locked to begin with")
	}
	app.Drag("id:wave", driver.Point{X: 300, Y: 150}, driver.Point{X: 260, Y: 150}, driver.Point{X: 220, Y: 150})
	app.Settle()
	if len(views) == 0 {
		t.Fatal("the drag did not move the view (no look-around)")
	}
	if !triggerLocked(app, src) {
		t.Fatal("the trigger did not come back after the drag lifted")
	}
}

func TestNoTriggerGesturesUnchanged(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(periodic(10000))
	var views []waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave", Series: []waveform.Series{{Source: src}},
			Follow: 1, OnViewChange: func(v waveform.Viewport) { views = append(views, v) },
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if len(views) == 0 || !views[len(views)-1].X.Valid() {
		t.Fatal("a zoom without a trigger no longer writes the view")
	}
	n := len(views)
	app.Drag("id:wave", driver.Point{X: 300, Y: 150}, driver.Point{X: 260, Y: 150}, driver.Point{X: 220, Y: 150})
	app.Settle()
	if len(views) <= n {
		t.Fatal("a pan without a trigger no longer writes the view")
	}
}
