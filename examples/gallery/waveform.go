package main

import (
	"math"
	"time"

base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	uiwave "github.com/nsgo-dev/nsgo/ui/waveform"
)

// waveSample is two channels of a 50 Hz tone with higher harmonics, as a
// device sampling at 20 kHz would read them.
func waveSample(i int, channel int) float32 {
	t := float64(i) / 20000
	// Every component is a whole multiple of 50 Hz, so each 20 ms cycle
	// is identical and the trigger locks on a steady trace.
	v := 1.2*math.Sin(2*math.Pi*50*t) + 0.4*math.Sin(2*math.Pi*450*t)
	if channel == 1 {
		v = 0.8*math.Sin(2*math.Pi*50*t+0.6) + 0.25*math.Sin(2*math.Pi*900*t)
	}
	return float32(v)
}

// waveformDemo shows a live scope: a second of samples to start, then a
// batch on every frame as a device thread publishes them. A rising trigger
// stands the waveform still, the time base sets a 50 ms window over the
// scope grid, and the readings follow the visible window.
func waveformDemo(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	sources := []*base.Source{
		base.NewSource(0, 1.0/20000),
		base.NewSource(0, 1.0/20000),
	}
	const seed = 20000 // a second
	for ch, src := range sources {
		y := make([]float32, seed)
		for i := range y {
			y[i] = waveSample(i, ch)
		}
		src.Push(y)
	}

	// A frame's batch at 20 kHz, published from the frame callback as a
	// device thread would publish it.
	from := seed
	var stop func()
	var loop func(time.Time)
	loop = func(time.Time) {
		// A small batch a frame keeps the live trace calm, about a fifth
		// of a 50 Hz cycle, so the trigger does not hop cycles every frame.
		const batch = 80
		for ch, src := range sources {
			y := make([]float32, batch)
			for i := range y {
				y[i] = waveSample(from+i, ch)
			}
			src.Push(y)
		}
		from += batch
		stop = cx.RequestAnimationFrame(loop)
	}
	stop = cx.RequestAnimationFrame(loop)
	cx.Cleanup(func() {
		if stop != nil {
			stop()
		}
	})

	run := &base.Run{}
	series := make([]base.Series, len(sources))
	for i, src := range sources {
		series[i] = base.Series{Source: src, Name: []string{"CH1", "CH2"}[i]}
	}
	// CH2 sits a little above CH1, as a channel's vertical position does.
	series[1].Offset = 2

	controls := core.View{
		Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter},
		Children: []core.Node{
			ui.Button{Text: tr.DemoWaveformRun(), Icon: lucide.Play,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Start},
			ui.Button{Text: tr.DemoWaveformStop(), Icon: lucide.Pause,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Stop},
			ui.Button{Text: tr.DemoWaveformSingle(), Icon: lucide.CircleDot,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Single},
		},
	}

	return core.View{
		Style: core.Style{Padding: core.All(20), Gap: 16},
		Children: []core.Node{
			core.Text{Content: tr.DemoWaveformDetail(), Style: core.Style{FontSize: 13}},
			controls,
			uiwave.Waveform{Chart: base.Chart{
				Series: series,
				Trigger: &base.Trigger{Source: 0, Level: 0, Edge: base.TriggerRising,
					Mode: base.TriggerAuto, Position: 0.4},
				Run:        run,
				TimeBase:   0.005, // 5 ms a division, 50 ms across ten
				DivisionsX: 10,
				DivisionsY: 8,
				Measurements: []base.Measurement{
					{Source: 0, Kind: base.MeasureVpp, Label: "Vpp"},
					{Source: 0, Kind: base.MeasureFrequency, Label: "f"},
					// Alt and a drag set the interval these read.
					{Source: 0, Kind: base.MeasureSpan, Label: "Δt", Over: base.MeasureMarkers},
					{Source: 0, Kind: base.MeasureVpp, Label: "Vpp [A–B]", Over: base.MeasureMarkers},
				},
				Cursor:     true,
				Legend:     true,
				Measure:    true,
				MarkLabels: [2]string{"A", "B"},
				Snap:       true,
				Height:     280,
				Label:      tr.DemoWaveform(),
				Style:      core.Style{Width: core.Pct(100)},
			}},
		},
	}
}
