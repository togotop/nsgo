package waveform_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func offsetChart(y []float32, offset float64) core.Node {
	return waveform.Chart{
		TestID: "wave",
		Series: []waveform.Series{{Y: y, X0: 0, Dx: 0.001, Offset: offset,
			Color: core.RGB(0x1a7f37)}},
		Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
	}
}

func TestChartChannelOffsetMovesTheAxis(t *testing.T) {
	y := waveSamples(1000)
	a := nsgotest.Start(t, func(cx *core.Scope) core.Node { return offsetChart(y, 0) }, nsgotest.Size(800, 600))
	a.Settle()
	b := nsgotest.Start(t, func(cx *core.Scope) core.Node { return offsetChart(y, 1000) }, nsgotest.Size(800, 600))
	b.Settle()
	if strings.Join(a.Texts(), "|") == strings.Join(b.Texts(), "|") {
		t.Fatal("a channel offset changed no axis label")
	}
}

func TestChartDivisionsDrawTheScopeGrid(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	chart := func(cols, rows int) core.Node {
		return waveform.Chart{
			TestID:     "wave",
			Series:     []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			DivisionsX: cols,
			DivisionsY: rows,
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}
	plain := nsgotest.Start(t, func(cx *core.Scope) core.Node { return chart(0, 0) }, nsgotest.Size(800, 600))
	plain.Settle()
	grid := nsgotest.Start(t, func(cx *core.Scope) core.Node { return chart(10, 8) }, nsgotest.Size(800, 600))
	grid.Settle()
	if a, b := len(plain.Find("id:wave-grid").Ops), len(grid.Find("id:wave-grid").Ops); b <= a {
		t.Fatalf("the scope grid added no line: %d ops -> %d", a, b)
	}
}

// Without a trigger, a time-based chart used to force its own window on
// every commit and ignore a zoom. A zoom must change what it shows, and
// the view must then keep that width as new samples arrive.
func TestChartTimeBaseDoesNotOverrideAZoom(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(20000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:     "wave",
			Series:     []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			TimeBase:   0.005,
			DivisionsX: 10,
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := strings.Join(app.Texts(), "|")
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if after := strings.Join(app.Texts(), "|"); after == before {
		t.Fatal("a zoom left the time-based view unchanged")
	}
}

// The bare arrow keys move the cursor; with Shift they pan the view, +/-
// zoom it and Shift+Home fits it again, as mygogo's plot does with its
// bare arrows and +/- keys.
func TestChartKeyboardMovesTheView(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(20000))
	var views []waveform.Viewport
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:       "wave",
			Series:       []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Cursor:       true,
			OnViewChange: func(v waveform.Viewport) { views = append(views, v) },
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600), nsgotest.Environment(driver.Environment{
		OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, Locale: "en-US", FontScale: 1, PixelRatio: 1}))
	app.Settle()
	app.Key("tab")
	app.Settle()
	for _, key := range []string{"shift+right", "equal", "minus", "shift+home"} {
		before := len(views)
		app.Key(key)
		app.Settle()
		if len(views) <= before {
			t.Fatalf("%s did not change the view", key)
		}
	}
}

// The app names the markers; the component shows only the words it gets.
func TestChartShowsMarkerLabels(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:     "wave",
			Series:     []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Measure:    true,
			MarkLabels: [2]string{"A", "B"},
			Style:      core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	for _, text := range []string{"text:A", "text:B"} {
		if !app.Exists(text) {
			t.Fatalf("no view shows %q", text)
		}
	}
}
