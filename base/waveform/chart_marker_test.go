package waveform_test

import (
	"fmt"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMarkersDoNotMoveAStoppedWaveform(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	run := &waveform.Run{}
	src.Push(waveSamples(20000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Run:    run,
			Trigger: &waveform.Trigger{Source: 0, Level: 250, Edge: waveform.TriggerRising,
				Position: 0.5},
			TimeBase:   0.005,
			DivisionsX: 10,
			Measure:    true,
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	app.Post(func() { run.Stop() })
	app.Settle()
	before := fmt.Sprint(app.Find("id:wave-s0").Ops)
	markBefore := app.Find("id:wave-mark-a").Frame.Origin.X
	// Marker A sits a third across the plot: 52 points of axis, 540 of plot.
	app.DragAt("id:wave", driver.Point{X: 52 + 180, Y: 100}, driver.Point{X: 40, Y: 0})
	app.Settle()
	if after := fmt.Sprint(app.Find("id:wave-s0").Ops); after != before {
		t.Fatal("moving a marker redrew the stopped waveform")
	}
	if markAfter := app.Find("id:wave-mark-a").Frame.Origin.X; markAfter == markBefore {
		t.Fatalf("the marker did not move: %v", markBefore)
	}
}
