package waveform_test

import (
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
