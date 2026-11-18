package waveform_test

import (
	"reflect"
	"strings"
	"testing"

	base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	uiwave "github.com/nsgo-dev/nsgo/ui/waveform"
)

func chartCanvases(app *nsgotest.App, selector string) []*nsgotest.View {
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

func TestWaveformRendersInTheTheme(t *testing.T) {
	y := make([]float32, 10000)
	for i := range y {
		y[i] = float32(200 + 110*float64(i%97)/97)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return uiwave.Waveform{Chart: base.Chart{
			TestID: "wave",
			Series: []base.Series{{Y: y, X0: 0, Dx: 0.001, Name: "CH1"}},
			Legend: true,
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}}
	}, nsgotest.Size(800, 600))
	app.Settle()
	cs := chartCanvases(app, "id:wave")
	if len(cs) == 0 || len(cs[0].Ops) < 10 {
		t.Fatal("the styled Waveform drew no line")
	}
	if !app.Exists("text:CH1") {
		t.Fatal("the styled Waveform lost the legend")
	}
}

func TestWaveformVariant(t *testing.T) {
	for _, v := range []uiwave.WaveformVariant{uiwave.WaveformNeutral, uiwave.WaveformAccent} {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return uiwave.Waveform{
				Variant: v,
				Chart: base.Chart{
					TestID: "wave",
					Series: []base.Series{{Y: []float32{1, 2, 3, 4}, X0: 0, Dx: 1}},
					Style:  core.Style{Width: core.Pt(400), Height: core.Pt(200)},
				},
			}
		}, nsgotest.Size(600, 400))
		app.Settle()
		if cs := chartCanvases(app, "id:wave"); len(cs) == 0 {
			t.Fatalf("variant %d mounted no canvas", v)
		}
	}
}

// TestStyledLayoutKeepsCanvasesFixed checks the boundary that keeps a
// waveform cheap: the canvas colors stay fixed in every theme, while the
// text around them gets fixed-width figures and theme classes that restyle
// in place without redrawing a canvas.
func TestStyledLayoutKeepsCanvasesFixed(t *testing.T) {
	w := uiwave.Waveform{Chart: base.Chart{
		Series: []base.Series{{Y: []float32{1, 2, 3, 4}, X0: 0, Dx: 1}},
	}}
	node := w.Render(nil)
	c, ok := node.(base.Chart)
	if !ok {
		t.Fatalf("Render returned %T, want base.Chart", node)
	}
	// Canvas colors are fixed values, never theme reads.
	if c.GridColor == 0 || c.TriggerColor == 0 || c.CrossLineStyle.Background == 0 || c.MarkStyle.Background == 0 {
		t.Fatalf("a canvas color was left unset: grid=%v trigger=%v cross=%v mark=%v",
			c.GridColor, c.TriggerColor, c.CrossLineStyle.Background, c.MarkStyle.Background)
	}
	if c.Series[0].Color == 0 {
		t.Fatal("a series color was left unset")
	}
	// Text fields are fixed-width and inherited by the views' children.
	if c.AxisStyle.FontVariant != core.FontTabularNums ||
		c.LegendStyle.FontVariant != core.FontTabularNums ||
		c.MeasurementStyle.FontVariant != core.FontTabularNums {
		t.Fatalf("the text around the chart is not tabular: axis=%v legend=%v measure=%v",
			c.AxisStyle.FontVariant, c.LegendStyle.FontVariant, c.MeasurementStyle.FontVariant)
	}
	if c.AxisClass != "text-muted-foreground" || c.LegendClass != "gap-3" || c.MeasurementClass == "" {
		t.Fatalf("the text views did not get their theme classes: axis=%q legend=%q measure=%q",
			c.AxisClass, c.LegendClass, c.MeasurementClass)
	}
	// The readings sit over a trace that redraws every frame. A backdrop
	// blur would make the browser re-blur that area on each of those frames.
	if strings.Contains(c.MeasurementClass, "backdrop") {
		t.Fatalf("the readings chip blurs its backdrop over a live trace: %q", c.MeasurementClass)
	}
	// An app's own class wins over the defaults.
	custom := uiwave.Waveform{Chart: base.Chart{
		Series:           []base.Series{{Y: []float32{1, 2}}},
		AxisClass:        "text-primary",
		MeasurementClass: "bg-transparent",
	}}
	if got := custom.Render(nil).(base.Chart); got.AxisClass != "text-primary" || got.MeasurementClass != "bg-transparent" {
		t.Fatalf("app classes were overwritten: axis=%q measure=%q", got.AxisClass, got.MeasurementClass)
	}
}

// TestThemeDoesNotRedrawCanvases proves the performance promise: switching
// appearance restyles the text views in place, but the canvases keep the
// same drawing operations because their colors are fixed, not theme reads.
func TestThemeDoesNotRedrawCanvases(t *testing.T) {
	app := func(dark bool) *nsgotest.App {
		return nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return uiwave.Waveform{Chart: base.Chart{
				TestID:  "wave",
				Series:  []base.Series{{Y: []float32{1, 2, 3, 4, 5, 6}, X0: 0, Dx: 1, Name: "CH1"}},
				Legend:  true,
				Measure: true,
				Style:   core.Style{Width: core.Pt(600), Height: core.Pt(300)},
			}}
		}, nsgotest.Size(800, 600), nsgotest.Environment(driver.Environment{Dark: dark}))
	}
	light, dark := app(false), app(true)
	lightOps, darkOps := chartCanvases(light, "id:wave"), chartCanvases(dark, "id:wave")
	if len(lightOps) == 0 || len(lightOps) != len(darkOps) {
		t.Fatalf("theme changed the canvas count: light=%d dark=%d", len(lightOps), len(darkOps))
	}
	for i := range lightOps {
		// DeepEqual compares a dash slice's contents, not the pointer
		// address that fmt prints — the two themes share the same drawing,
		// only in different allocations.
		if !reflect.DeepEqual(darkOps[i].Ops, lightOps[i].Ops) {
			t.Fatalf("canvas %d redrew differently in the dark theme:\nlight %v\ndark  %v", i, lightOps[i].Ops, darkOps[i].Ops)
		}
	}
}
