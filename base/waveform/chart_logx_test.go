package waveform_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestChartLogXAxisDrawsDecadeTicks(t *testing.T) {
	y := make([]float32, 1000)
	for i := range y {
		y[i] = 1
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID: "wave",
			Series: []waveform.Series{{Y: y, X0: 1, Dx: 1, Color: core.RGB(0x1a7f37)}},
			XLog:   true,
			Style:  core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if !app.Exists("text:100") {
		t.Fatalf("a log x axis drew no decade label; tree:\n%s", app.Dump())
	}
}

// On a log x axis the pointer in the middle of the plot must report the
// geometric middle of the view, not its arithmetic middle: a linear
// mapping would put the cursor near 500 instead of about 31.6.
func TestChartLogXCursorUsesTheViewScale(t *testing.T) {
	var got []float64
	y := make([]float32, 1000)
	for i := range y {
		y[i] = 1
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return waveform.Chart{
			TestID:         "wave",
			Series:         []waveform.Series{{Y: y, X0: 1, Dx: 1, Color: core.RGB(0x1a7f37)}},
			XLog:           true,
			Cursor:         true,
			OnCursorChange: func(x float64) { got = append(got, x) },
			Style:          core.Style{Width: core.Pt(600), Height: core.Pt(300)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	// The plot starts 52 points in and is 540 wide: its middle is at 322.
	app.HoverAt("id:wave", driver.Point{X: 322, Y: 100})
	app.Settle()
	if len(got) == 0 {
		t.Fatal("the cursor reported no x")
	}
	x := got[len(got)-1]
	if math.IsNaN(x) || math.Abs(x-math.Sqrt(1000)) > 3 {
		t.Fatalf("the middle of a 1..1000 log view read %v, want about 31.6", x)
	}
}
