package waveform_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func waveSamples(n int) []float32 {
	y := make([]float32, n)
	for i := range y {
		y[i] = float32(200 + 110*math.Sin(float64(i)/37) + 45*math.Sin(float64(i)/3.1))
	}
	return y
}

func canvases(app *nsgotest.App, selector string) []*nsgotest.View {
	var out []*nsgotest.View
	var walk func(*nsgotest.View)
	walk = func(v *nsgotest.View) {
		if v == nil {
			return
		}
		if v.Kind == driver.KindCanvas {
			out = append(out, v)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(app.Find(selector))
	return out
}

func startChart(t *testing.T, src *waveform.Source, n int) (*nsgotest.App, []*nsgotest.View) {
	t.Helper()
	if src != nil {
		src.Push(waveSamples(n))
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	return app, canvases(app, "id:wave")
}

func TestChartDrawsDecimatedLine(t *testing.T) {
	app, _ := startChart(t, waveform.NewSource(0, 0.001), 100000)
	ops := len(app.Find("id:wave-s0").Ops)
	if ops < 10 {
		t.Fatalf("the series drew %d ops, want a line", ops)
	}
	if ops > 3000 {
		t.Fatalf("the series drew %d ops, want about twice the plot's width", ops)
	}
}

func TestChartOpsDoNotGrowWithSamples(t *testing.T) {
	var first int
	for _, n := range []int{100000, 1000000} {
		app, _ := startChart(t, waveform.NewSource(0, 0.001), n)
		ops := len(app.Find("id:wave-s0").Ops)
		if ops > 3000 {
			t.Fatalf("%d samples: %d ops, want about twice the width", n, ops)
		}
		if first == 0 {
			first = ops
		} else if ops > first*2 {
			t.Fatalf("%d samples: %d ops against %d at 100k, not O(width)", n, ops, first)
		}
	}
}

func TestSourcePushFromAnotherGoroutine(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	done := make(chan struct{})
	go func() {
		src.Push(waveSamples(100000))
		close(done)
	}()
	<-done
	app.Settle()
	cs := canvases(app, "id:wave")
	if len(cs) == 0 || len(cs[0].Ops) < 10 {
		t.Fatal("a push from another goroutine never reached the drawing")
	}
}

func TestChartCursorReportsTheDataX(t *testing.T) {
	var got []float64
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:         "wave",
			Series:         []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Cursor:         true,
			OnCursorChange: func(x float64) { got = append(got, x) },
			Style:          core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.HoverAt("id:wave", driver.Point{X: 300, Y: 100})
	app.Settle()
	if len(got) == 0 || math.IsNaN(got[len(got)-1]) {
		t.Fatalf("the cursor reported no data x: %v", got)
	}
	app.HoverOut()
	app.Settle()
	if last := got[len(got)-1]; !math.IsNaN(last) {
		t.Fatalf("leaving the chart did not clear the cursor: %v", last)
	}
}

func TestChartWheelZoomReportsTheView(t *testing.T) {
	var views []waveform.Viewport
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
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
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if len(views) <= before || !views[len(views)-1].X.Valid() {
		t.Fatalf("a wheel zoom did not report a view: %v", views)
	}
}

func TestChartGroupSharesTheView(t *testing.T) {
	g := &waveform.Group{}
	var bottomViews []waveform.Viewport
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	chart := func(id string, onChange func(waveform.Viewport)) core.Node {
		return waveform.Chart{
			TestID:       id,
			Sync:         g,
			Series:       []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			OnViewChange: onChange,
			Style:        core.Style{Width: core.Pt(600), Height: core.Pt(200)},
		}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			chart("top", nil),
			chart("bottom", func(v waveform.Viewport) { bottomViews = append(bottomViews, v) }),
		}}
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := len(bottomViews)
	app.Wheel("id:top", driver.Point{Y: -100}, driver.Modifiers{Control: true})
	app.Settle()
	if len(bottomViews) <= before {
		t.Fatal("a zoom in one chart did not reach the other of the group")
	}
	for _, v := range bottomViews[before:] {
		if v.X.Valid() {
			return
		}
	}
	t.Fatal("the group's new view was empty")
}

func TestChartLegendNotesAndBands(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Name: "CH1", Color: core.RGB(0x1a7f37)}},
			Legend: true,
			Notes:  []waveform.Note{{X: 2, Y: 250, Text: "trigger"}},
			Bands:  []waveform.Band{{Lo: 100, Hi: 300, Label: "safe"}},
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	for _, text := range []string{"text:CH1", "text:trigger", "text:safe"} {
		if !app.Exists(text) {
			t.Errorf("no view shows %q", text)
		}
	}
}

func TestMeasureMarkersHoldTheirPlaceWhileFollowing(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:  "wave",
			Series:  []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Follow:  1,
			Measure: true,
			Style:   core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	before := app.Find("id:wave-mark-a").Frame.Origin.X
	app.Post(func() { src.Push(waveSamples(2000)) })
	app.Settle()
	after := app.Find("id:wave-mark-a").Frame.Origin.X
	if before != after {
		t.Fatalf("a following window moved the marker: %v -> %v", before, after)
	}
}

func TestChartFormatsItsXTicks(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(10000), X0: 0, Dx: 0.001,
				Color: core.RGB(0x1a7f37)}},
			XFormat: func(v float64) string {
				return "T" + strconv.FormatFloat(v, 'f', 1, 64)
			},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !app.Exists("text:T") {
		t.Fatal("the x axis did not use the app's formatter")
	}
}

func TestChartLayersASeriesPerCanvas(t *testing.T) {
	var styled int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{
				{Y: waveSamples(100), X0: 0, Dx: 0.001, Class: "one",
					StyleFunc: func() core.Style { styled++; return core.Style{} }},
				{Y: waveSamples(100), X0: 0, Dx: 0.001, Class: "two"},
			},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if styled == 0 {
		t.Fatal("a series' StyleFunc never ran, so its canvas does not take it")
	}
	for _, id := range []string{"id:wave-grid", "id:wave-s0", "id:wave-s1", "id:wave-notes"} {
		if !app.Exists(id) {
			t.Errorf("no canvas %s", id)
		}
	}
}

func TestChartLogAxis(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: []float32{1, 10, 100, 1000}, X0: 0, Dx: 1,
				Color: core.RGB(0x1a7f37)}},
			Axes:  []waveform.Axis{{Log: true}},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !app.Exists("id:wave-y0-0") {
		t.Fatal("a log axis drew no labels")
	}
}

func TestChartTooltipFollowsTheCursor(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(1000), X0: 0, Dx: 0.001,
				Color: core.RGB(0x1a7f37)}},
			Cursor:  true,
			Tooltip: core.View{TestID: "tip", Style: core.Style{Width: core.Pt(60), Height: core.Pt(20)}},
			Style:   core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.HoverAt("id:wave", driver.Point{X: 300, Y: 100})
	app.Settle()
	if !app.Exists("id:wave-cross") {
		t.Fatalf("no cross line with the cursor; tree:\n%s", app.Dump())
	}
	if !app.Exists("id:tip") {
		t.Fatalf("the tooltip did not show with the cursor; tree:\n%s", app.Dump())
	}
}

func TestChartAltWheelZoomsTheYAxis(t *testing.T) {
	app, _ := startChart(t, waveform.NewSource(0, 0.001), 10000)
	before := strings.Join(app.Texts(), "|")
	app.Wheel("id:wave", driver.Point{Y: -100}, driver.Modifiers{Alt: true})
	app.Settle()
	if after := strings.Join(app.Texts(), "|"); after == before {
		t.Fatal("Alt and the wheel changed no label")
	}
}

func TestChartGroupSharesTheCursor(t *testing.T) {
	g := &waveform.Group{}
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	chart := func(id string) core.Node {
		return waveform.Chart{TestID: id, Sync: g, Cursor: true,
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(200)}}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{chart("top"), chart("bottom")}}
	}, nsgotest.Size(800, 600))
	app.HoverAt("id:top", driver.Point{X: 300, Y: 60})
	app.Settle()
	if !app.Exists("id:bottom-cross") {
		t.Fatal("the group's cursor did not reach the other chart")
	}
}

func TestChartSeriesLineWidthFollowsTheView(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(100), X0: 0, Dx: 0.001,
				Style: core.Style{StrokeWidth: 4}}},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	v := app.Find("id:wave-s0")
	if v.Drawing == nil {
		t.Fatal("the series drew nothing")
	}
	for _, p := range v.Drawing.ViewPaints {
		if p.Kind == driver.ViewStrokeWidth {
			return
		}
	}
	t.Fatal("the series' line width is not taken from its view")
}

func TestChartRightAxisSitsRightOfTheLeft(t *testing.T) {
	y := waveSamples(1000)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{
				{Y: y, X0: 0, Dx: 0.001, Axis: 0, Color: core.RGB(0x1a7f37)},
				{Y: y, X0: 0, Dx: 0.001, Axis: 1, Color: core.RGB(0xcc3333)},
			},
			Axes:  []waveform.Axis{{}, {}},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	left := app.Find("id:wave-y0-0").Frame.Origin.X
	right := app.Find("id:wave-y1-0").Frame.Origin.X
	if right <= left {
		t.Fatalf("the right axis label at %v is not right of the left one at %v", right, left)
	}
}

func TestChartTooltipKeepsItsOffset(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(10000), X0: 0, Dx: 0.001,
				Color: core.RGB(0x1a7f37)}},
			Cursor:        true,
			Tooltip:       core.View{TestID: "tip", Style: core.Style{Width: core.Pt(60), Height: core.Pt(20)}},
			TooltipOffset: 8,
			Style:         core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.HoverAt("id:wave", driver.Point{X: 300, Y: 100})
	app.Settle()
	cross := app.Find("id:wave-cross").Frame.Origin.X
	box := app.Find("id:wave-tipbox").Frame.Origin.X
	if d := box - cross; d < 6 || d > 10 {
		t.Fatalf("the tooltip is %v from the cross line, want about 8", d)
	}
}

func TestChartNotesSitAtTheirX(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(10000), X0: 0, Dx: 0.001,
				Color: core.RGB(0x1a7f37)}},
			Notes: []waveform.Note{{X: 2, Y: 250, Text: "trigger"}},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	x := app.Find("text:trigger").Frame.Origin.X
	if x < 80 || x > 140 {
		t.Fatalf("the note at data x 2 sits at %v, want about 108", x)
	}
}

func TestChartLabelsEachYAxis(t *testing.T) {
	y := waveSamples(10000)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{
				{Y: y, X0: 0, Dx: 0.001, Axis: 0, Color: core.RGB(0x1a7f37)},
				{Y: y, X0: 0, Dx: 0.001, Axis: 1, Color: core.RGB(0xcc3333)},
				{Y: y, X0: 0, Dx: 0.001, Axis: 2, Color: core.RGB(0x3333cc)},
			},
			Axes:  []waveform.Axis{{Title: "V"}, {Title: "mV"}, {Title: "A"}},
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	for _, id := range []string{"id:wave-y0-0", "id:wave-y1-0", "id:wave-y2-0",
		"id:wave-y0-title", "id:wave-y1-title", "id:wave-y2-title"} {
		if !app.Exists(id) {
			t.Errorf("no label or title for the y axis %s", id)
		}
	}
}

func TestChartKeysMoveAndClearTheCursor(t *testing.T) {
	var got []float64
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(10000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:         "wave",
			Series:         []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Cursor:         true,
			OnCursorChange: func(x float64) { got = append(got, x) },
			Style:          core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600), nsgotest.Environment(driver.Environment{
		OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, Locale: "en-US", FontScale: 1, PixelRatio: 1}))
	app.Key("tab")
	app.Settle()
	app.Key("right")
	app.Settle()
	if len(got) == 0 || math.IsNaN(got[len(got)-1]) {
		t.Fatalf("Right did not place a cursor: %v", got)
	}
	app.Key("home")
	app.Settle()
	if last := got[len(got)-1]; last != 0 {
		t.Fatalf("Home did not take the cursor to the view's start: %v", last)
	}
	app.Key("end")
	app.Settle()
	if last := got[len(got)-1]; last <= 0 {
		t.Fatalf("End did not take the cursor to the view's end: %v", last)
	}
	app.Key("escape")
	app.Settle()
	if last := got[len(got)-1]; !math.IsNaN(last) {
		t.Fatalf("Escape did not clear the cursor: %v", last)
	}
}

func TestChartNoteIsItsAccessibilityValue(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: waveSamples(1000), X0: 0, Dx: 0.001,
				Color: core.RGB(0x1a7f37)}},
			Label: "Channels",
			Note:  "15 s at 20 kHz",
			Style: core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if got := app.Find("id:wave").Props.Value.Text; got != "15 s at 20 kHz" {
		t.Fatalf("the chart's note was not its accessibility value: %q", got)
	}
}

func TestChartFollowPinsNewestSamples(t *testing.T) {
	src := waveform.NewSource(0, 0.001)
	src.Push(waveSamples(100000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Source: src, Color: core.RGB(0x1a7f37)}},
			Follow: 1, // one x unit, a tenth of the column
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	cs := canvases(app, "id:wave")
	if len(cs) == 0 || len(cs[0].Ops) < 10 {
		t.Fatal("a following chart drew nothing")
	}
}
