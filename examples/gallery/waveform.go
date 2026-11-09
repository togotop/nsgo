package main

import (
	"math"
	"time"

	base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	uiwave "github.com/nsgo-dev/nsgo/ui/waveform"
)

// waveSample is two channels of a 50 Hz tone with higher harmonics, as a
// device sampling at 20 kHz would read them.
func waveSample(i int, channel int) float32 {
	t := float64(i) / 20000
	v := 1.2*math.Sin(2*math.Pi*50*t) + 0.4*math.Sin(2*math.Pi*430*t)
	if channel == 1 {
		v = 0.8*math.Sin(2*math.Pi*50*t+0.6) + 0.25*math.Sin(2*math.Pi*900*t)
	}
	return float32(v)
}

// waveformDemo shows a live waveform: a second of samples to start, then
// a batch of new ones on every frame, as a device thread publishes them.
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
		const batch = 333
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

	series := make([]base.Series, len(sources))
	for i, src := range sources {
		series[i] = base.Series{Source: src, Name: []string{"CH1", "CH2"}[i]}
	}
	return core.View{
		Style: core.Style{Padding: core.All(20), Gap: 16},
		Children: []core.Node{
			core.Text{Content: tr.DemoWaveformDetail(), Style: core.Style{FontSize: 13}},
			uiwave.Waveform{Chart: base.Chart{
				Series:  series,
				Follow:  0.05, // a 50 ms window of the newest samples
				Cursor:  true,
				Legend:  true,
				Measure: true,
				Height:  280,
				Label:   tr.DemoWaveform(),
				Style:   core.Style{Width: core.Pct(100)},
			}},
		},
	}
}
