package waveform

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A chart with neither Follow nor TimeBase shows every sample it has. Its
// source must still be bounded, or a live source grows without limit.
func TestChartBoundsAnIdleSource(t *testing.T) {
	src := NewSource(0, 0.001)
	src.Push(make([]float32, 1000))
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return Chart{
			Series: []Series{{Source: src}},
			Style:  core.Style{Width: core.Pt(400), Height: core.Pt(200)},
		}
	}, nsgotest.Size(800, 600))
	app.Settle()
	if want := defaultRetainSamples * 0.001; src.retain != want {
		t.Fatalf("an idle chart left its source unbounded: retain %v, want %v", src.retain, want)
	}
}

func TestRetainWindow(t *testing.T) {
	if got := (Chart{}).retainWindow(0.001); got != defaultRetainSamples*0.001 {
		t.Fatalf("an idle chart's window: %v", got)
	}
	if got := (Chart{TimeBase: 0.005, DivisionsX: 10}).retainWindow(0.001); got != retainFactor*0.05 {
		t.Fatalf("a time-based chart's window: %v", got)
	}
	if got := (Chart{Follow: 2}).retainWindow(0.001); got != retainFactor*2 {
		t.Fatalf("a following chart's window: %v", got)
	}
}
