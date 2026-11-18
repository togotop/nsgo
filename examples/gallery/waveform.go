package main

import (
	"math"
	"strconv"
	"strings"
	"time"

	base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	uiwave "github.com/nsgo-dev/nsgo/ui/waveform"
)

// siPrefixes are the engineering steps a scope reading takes, yocto to yotta.
var siPrefixes = map[int]string{
	-24: "y", -21: "z", -18: "a", -15: "f", -12: "p", -9: "n", -6: "µ", -3: "m",
	0: "", 3: "k", 6: "M", 9: "G", 12: "T", 15: "P", 18: "E", 21: "Z", 24: "Y",
}

// si writes v with unit as a scope reads it out: 1.25 V, 4.00 ms, 15.6 MHz.
// Three significant digits keep a reading steady enough to compare.
func si(v float64, unit string) string {
	switch {
	case math.IsNaN(v):
		return "—"
	case math.IsInf(v, 0):
		return "∞"
	case v == 0:
		if unit == "" {
			return "0"
		}
		return "0 " + unit
	}
	exp := int(math.Floor(math.Log10(math.Abs(v))/3)) * 3
	exp = max(-24, min(24, exp))
	scaled := v / math.Pow(10, float64(exp))
	text := strconv.FormatFloat(scaled, 'g', 3, 64)
	if strings.Contains(text, "e") {
		// Rounding pushed the value into the next step, 999.5 to 1e+03.
		exp += 3
		text = strconv.FormatFloat(scaled/1000, 'g', 3, 64)
	}
	text += " " + siPrefixes[exp]
	if unit != "" {
		text += unit
	}
	return strings.TrimSpace(text)
}

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

	// Samples reach the chart at a fixed rate in wall time, as a device
	// would clock them, not a fixed number per frame: a frame callback can
	// run at any rate, and a per-frame batch would make the trace race on a
	// fast display. pace is seconds of captured data per second of wall
	// time, so the 50 ms window glides by in about a second whatever the
	// frame rate; carry holds the fraction of a sample owed to the next
	// frame. The trigger still pins a rising edge while the view is held.
	const sampleRate = 20000
	const pace = 0.05
	from := seed
	var carry float64
	var since time.Time
	var stop func()
	var loop func(time.Time)
	loop = func(at time.Time) {
		if since.IsZero() {
			since = at
		}
		carry += at.Sub(since).Seconds() * sampleRate * pace
		since = at
		batch := int(carry)
		carry -= float64(batch)
		if batch > 0 {
			for ch, src := range sources {
				y := make([]float32, batch)
				for i := range y {
					y[i] = waveSample(from+i, ch)
				}
				src.Push(y)
			}
			from += batch
		}
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

	// A/B anchors and how they read, as a scope's cursors do: vertical
	// lines reading the time span (Δt), or horizontal ones reading the
	// value difference (Δ=). The buttons switch the marking, and the
	// chart and its measurements follow the signal.
	markA, markB := core.NewSignal(cx, 0.3), core.NewSignal(cx, 0.7)
	reading := core.NewSignal(cx, base.MarkReadingTime)
	// A scope's trigger holds a repeating signal still; without one the
	// trace scrolls by as samples arrive. It watches a chosen channel's
	// rising edge across a level; each channel keeps its own level so
	// switching the source does not lose the setting.
	triggerOn := core.NewSignal(cx, true)
	triggerSource := core.NewSignal(cx, 0)
	triggerLevels := [2]*core.Signal[float64]{core.NewSignal(cx, 0.0), core.NewSignal(cx, 0.0)}
	// A pan hands the view to the user, so the trigger stands down even
	// while its switch stays on. Re-arm rebuilds the chart, which restores
	// the empty view and lets the trigger take hold again.
	rearm := core.NewSignal(cx, 0)
	// The chart owns a zoom or pan while it runs; keep a copy here so a
	// rebuild from a trigger change starts from the same view instead of
	// losing the zoom. Re-arm clears it to put the trigger back in charge.
	savedView := core.NewSignal(cx, base.Viewport{})
	markBtn := func(text string, mode base.MarkReading) core.Node {
		variant := ui.ButtonGhost
		if reading.Get() == mode {
			variant = ui.ButtonSecondary
		}
		return ui.Button{Text: text, Variant: variant, Size: ui.SizeSmall,
			OnPress: func() { reading.Set(mode) }}
	}

	runControls := core.View{
		Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Wrap: core.Wrap},
		Children: []core.Node{
			ui.Button{Text: tr.DemoWaveformRun(), Icon: lucide.Play,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Start},
			ui.Button{Text: tr.DemoWaveformStop(), Icon: lucide.Pause,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Stop},
			ui.Button{Text: tr.DemoWaveformSingle(), Icon: lucide.CircleDot, TestID: "wave-single",
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: run.Single},
			ui.Button{Text: tr.DemoWaveformRearm(), Icon: lucide.RotateCcw,
				Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
				OnPress: func() {
					savedView.Set(base.Viewport{})
					rearm.Update(func(n int) int { return n + 1 })
				}},
			core.Dynamic(func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, Gap: 4, AlignItems: core.AlignCenter},
					Children: []core.Node{
						markBtn(tr.DemoWaveformMarkTime(), base.MarkReadingTime),
						markBtn(tr.DemoWaveformMarkValue(), base.MarkReadingValue),
					}}
			}),
		},
	}
	triggerControls := core.View{
		Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, Wrap: core.Wrap},
		Children: []core.Node{
			ui.Switch{Size: ui.SizeSmall, Label: tr.DemoWaveformTrigger(), TestID: "wave-trigger",
				Checked: triggerOn.Get, OnChange: triggerOn.Set},
			core.Dynamic(func(*core.Scope) core.Node {
				return ui.Tabs[int]{Size: ui.SizeSmall,
					Options: []ui.Option[int]{{Value: 0, Label: "CH1"}, {Value: 1, Label: "CH2"}},
					Value:   triggerSource.Get, OnChange: triggerSource.Set}
			}),
			core.Dynamic(func(*core.Scope) core.Node {
				ch := triggerSource.Get()
				level, span := triggerLevels[ch], 1.6
				if ch == 1 {
					span = 1.05
				}
				return numberKnob(tr.DemoWaveformTriggerLevel(), level, -span, span, 0.05)
			}),
		},
	}
	controls := core.View{
		Style:    core.Style{Direction: core.Column, Gap: 8},
		Children: []core.Node{runControls, triggerControls},
	}

	// The readings carry a scope's engineering units, and the markers'
	// own reading follows the chosen orientation: Δt in time mode, Δ=
	// (the value difference on CH1) in value mode.
	readings := func() []base.Measurement {
		m := []base.Measurement{
			{Source: 0, Kind: base.MeasureVpp, Label: "Vpp", Format: func(v float64) string { return si(v, "V") }},
			{Source: 0, Kind: base.MeasureFrequency, Label: "f", Format: func(v float64) string { return si(v, "Hz") }},
		}
		if reading.Get() == base.MarkReadingValue {
			return append(m, base.Measurement{Source: 0, Kind: base.MeasureMarkDelta, Label: "Δ=",
				Format: func(v float64) string { return si(v, "V") }})
		}
		return append(m,
			base.Measurement{Source: 0, Kind: base.MeasureSpan, Label: "Δt", Over: base.MeasureMarkers,
				Format: func(v float64) string { return si(v, "s") }},
			base.Measurement{Source: 0, Kind: base.MeasureVpp, Label: "Vpp [A–B]", Over: base.MeasureMarkers,
				Format: func(v float64) string { return si(v, "V") }},
		)
	}

	return core.View{
		Style: core.Style{Padding: core.All(20), Gap: 16},
		Children: []core.Node{
			core.Text{Content: tr.DemoWaveformDetail(), Style: core.Style{FontSize: 13}},
			core.Text{Content: tr.DemoWaveformZoomHint(), StyleFunc: func() core.Style {
				return core.Style{FontSize: 11, Color: ui.ColorsOf(cx).Muted()}
			}},
			controls,
			core.Dynamic(func(*core.Scope) core.Node {
				// Reading rearm makes this dynamic rebuild when Re-arm is
				// pressed, which restores the empty view so the trigger
				// takes hold again after a pan.
				rearm.Get()
				// No trigger means the trace scrolls by as samples arrive;
				// with one it locks on the chosen channel's rising edge at
				// that channel's level.
				var trigger *base.Trigger
				if triggerOn.Get() {
					ch := triggerSource.Get()
					trigger = &base.Trigger{Source: ch, Level: triggerLevels[ch].Get(), Edge: base.TriggerRising,
						Mode: base.TriggerAuto, Position: 0.4}
				}
				return uiwave.Waveform{Chart: base.Chart{
					Series:  series,
					Trigger: trigger,
					Run:     run,
					// Follow the newest 200 ms, about ten 50 Hz cycles,
					// so the default view shows many peaks. A Follow
					// window, not a TimeBase, keeps Single's arm-and-stop
					// working: TimeBase pins the newest sample beyond the
					// edge search, so no new edge can ever pass the arm.
					Follow:     0.2,
					DivisionsX: 10,
					DivisionsY: 8,
					// Start from the view the user kept, so changing the
					// trigger does not undo a zoom or pan.
					View:         savedView.Get(),
					OnViewChange: savedView.Set,
					Axes: []base.Axis{
						{Title: "CH1", Format: func(v float64) string { return si(v, "V") }},
						{Title: "CH2", Format: func(v float64) string { return si(v, "V") }},
					},
					XFormat:      func(v float64) string { return si(v, "s") },
					Measurements: readings(),
					Cursor:       true,
					Legend:       true,
					Measure:      true,
					MarkLabels:   [2]string{"A", "B"},
					MarkReading:  reading.Get(),
					MarkA:        markA,
					MarkB:        markB,
					MarkChannel:  0,
					Snap:         true,
					Height:       280,
					// Readings sit in a fixed-pitch face on a rounded chip, so a
					// trace behind them stays clear of the digits and they do
					// not shift as they change. No theme colour here: the chart
					// is built in a Dynamic that follows the marking mode, and
					// reading the palette in it would rebuild the chart when the
					// appearance changes, where a gallery demo restyles in place.
					MeasurementStyle: core.Style{Padding: core.All(6), CornerRadius: 6,
						FontFamily: core.FontMonospace},
					TestID: "wave",
					Label:  tr.DemoWaveform(),
					Style:  core.Style{Width: core.Pct(100)},
				}}
			}),
		},
	}
}
