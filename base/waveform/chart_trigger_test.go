package waveform_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func triggerChart(src *waveform.Source, run *waveform.Run, tr *waveform.Trigger) core.Node {
	return waveform.Chart{
		TestID:  "wave",
		Series:  []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
		Trigger: tr,
		Run:     run,
		Follow:  1,
		Style:   core.Style{Width: core.Pt(600), Height: core.Pt(300)},
	}
}

func TestChartRunStopsAtTheLastFrame(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	run := &waveform.Run{}
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return triggerChart(src, run, nil)
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := len(app.Find("id:wave-s0").Ops)
	beforeTexts := strings.Join(app.Texts(), "|")
	app.Post(func() { run.Stop() })
	app.Settle()
	app.Post(func() { src.Push(waveSamples(5000)) })
	app.Settle()
	if after := len(app.Find("id:wave-s0").Ops); after != before {
		t.Fatalf("a stopped chart redrew: %d ops -> %d", before, after)
	}
	if run.Running() {
		t.Fatal("the run did not stop")
	}
	frozen := strings.Join(app.Texts(), "|")
	if frozen != beforeTexts {
		t.Fatal("a stopped chart changed its labels")
	}
	// Starting again must wake the memo it left holding the last frame.
	app.Post(func() { run.Start() })
	app.Post(func() { src.Push(waveSamples(5000)) })
	app.Settle()
	if !run.Running() {
		t.Fatal("Run did not resume")
	}
	if resumed := strings.Join(app.Texts(), "|"); resumed == frozen {
		t.Fatal("a resumed chart did not redraw")
	}
}

func TestChartSingleStopsAfterAnEdge(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	run := &waveform.Run{}
	tr := &waveform.Trigger{Source: 0, Level: 250, Edge: waveform.TriggerRising}
	src.Push(waveSamples(3000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return triggerChart(src, run, tr)
	}, nsgotest.Size(800, 600))
	app.Settle()
	app.Post(func() { run.Single() })
	app.Settle()
	app.Post(func() { src.Push(waveSamples(2000)) })
	app.Settle()
	if run.Running() {
		t.Fatal("a single capture did not stop after its edge")
	}
}

// Single must wait for an edge that arrives after the press, not stop on
// one already sitting in the buffer.
func TestChartSingleWaitsForANewEdge(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	run := &waveform.Run{}
	tr := &waveform.Trigger{Source: 0, Level: 250, Edge: waveform.TriggerRising}
	src.Push(waveSamples(3000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return triggerChart(src, run, tr)
	}, nsgotest.Size(800, 600))
	app.Settle()
	app.Post(func() { run.Single() })
	app.Settle()
	if !run.Running() {
		t.Fatal("Single stopped on an edge that was already in the buffer")
	}
}

func TestChartTriggerDrawsItsEdge(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	plain := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return triggerChart(src, nil, nil)
	}, nsgotest.Size(800, 600))
	plain.Settle()
	with := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return triggerChart(src, nil, &waveform.Trigger{Source: 0, Level: 250, Edge: waveform.TriggerRising})
	}, nsgotest.Size(800, 600))
	with.Settle()
	if a, b := len(plain.Find("id:wave-grid").Ops), len(with.Find("id:wave-grid").Ops); b <= a {
		t.Fatalf("the trigger added no line: %d ops -> %d", a, b)
	}
}

func TestChartStaysInteractiveWhenStopped(t *testing.T) {
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
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	app.Post(func() { run.Stop() })
	app.Settle()
	before := strings.Join(app.Texts(), "|")
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if after := strings.Join(app.Texts(), "|"); after == before {
		t.Fatal("a stopped chart did not zoom over its capture")
	}
}

// A single-shot trigger captures one view and holds it; later samples must
// not re-anchor the waveform.
func TestTriggerSingleHoldsItsCapture(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(20000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Trigger: &waveform.Trigger{Source: 0, Level: 250, Edge: waveform.TriggerRising,
				Mode: waveform.TriggerSingle, Position: 0.5},
			TimeBase:   0.005,
			DivisionsX: 10,
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := fmt.Sprint(app.Find("id:wave-s0").Ops)
	app.Post(func() { src.Push(waveSamples(5000)) })
	app.Settle()
	if after := fmt.Sprint(app.Find("id:wave-s0").Ops); after != before {
		t.Fatal("a single-shot trigger re-anchored on a later edge")
	}
}
