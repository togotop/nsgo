package waveform

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// dragState is a press in progress: a drag selects a range to zoom to,
// Shift and a drag pans, and a press near a measurement marker moves it.
type dragState struct {
	active bool
	pan    bool
	marker int // 0: none, 1: A, 2: B
	fromX  float64
	toX    float64
	view   Viewport
}

// input is a Chart's interactive state for one render. It turns the
// pointer's place in the root view into a data x, and changes the view,
// the cursor and the markers through signals, so every handler leads to
// one commit.
type input struct {
	c        Chart
	view     *core.Signal[Viewport]
	hover    *core.Signal[float64]
	fade     *core.AnimatedValue
	markA    *core.Signal[float64]
	markB    *core.Signal[float64]
	yZoom    *core.Signal[float64]
	plotSize *core.Signal[core.Size]
	xView    func() Range
	setView  func(Viewport)
	drag     dragState
}

func newInput(c Chart, view *core.Signal[Viewport], hover, markA, markB, yZoom *core.Signal[float64],
	fade *core.AnimatedValue, plotSize *core.Signal[core.Size], xView func() Range, setView func(Viewport)) *input {
	return &input{c: c, view: view, hover: hover, fade: fade, markA: markA, markB: markB,
		yZoom: yZoom, plotSize: plotSize, xView: xView, setView: setView}
}

// ownView is the view this chart shows, fitted and clamped to the data,
// as xView reports it: a zoom or a pan starts from what the user sees,
// not from the uninitialized viewport a Chart begins with.
func (in *input) ownView() Viewport { return Viewport{X: in.xView()} }

// xAt turns a pointer's x in the root view into a data x. The plot is
// inset by axisRoom on the left, and its width is what the canvas reports.
func (in *input) xAt(px float64) float64 {
	w := float64(in.plotSize.Get().Width)
	if w <= 0 {
		return 0
	}
	f := min(max((px-float64(axisRoom))/w, 0), 1)
	vx := in.xView()
	return vx.Lo + f*vx.Span()
}

func (in *input) slop() float64 { return max(in.xView().Span()*0.02, 1e-9) }

// frac is where a data x lies in the view, 0 to 1. Markers are kept as
// fractions so they hold their place on screen as the window moves.
func (in *input) frac(x float64) float64 {
	vx := in.xView()
	if vx.Span() <= 0 {
		return 0
	}
	return min(max((x-vx.Lo)/vx.Span(), 0), 1)
}

func (in *input) onMouseMove(e core.MouseEvent) {
	x := in.xAt(float64(e.Position.X))
	if in.drag.active {
		in.drag.toX = x
		switch {
		case in.drag.pan:
			in.setView(in.drag.view.PanX(in.drag.fromX - x))
		case in.drag.marker == 1:
			in.markA.Set(in.frac(x))
		case in.drag.marker == 2:
			in.markB.Set(in.frac(x))
		}
		return
	}
	if e.PressedButton != core.MouseButtonNone || (!in.c.Cursor && !in.c.Measure) {
		return
	}
	in.setCursor(x)
}

// setCursor and clearCursor are the one path every input takes to the
// cursor: the pointer, the keys and the screen reader's actions alike.
func (in *input) setCursor(x float64) {
	in.hover.Set(x)
	in.fade.Set(1)
	in.c.Sync.setCursor(x)
	if in.c.OnCursorChange != nil {
		in.c.OnCursorChange(x)
	}
}

func (in *input) clearCursor() {
	in.hover.Set(math.NaN())
	in.fade.Set(0)
	in.c.Sync.setCursor(math.NaN())
	if in.c.OnCursorChange != nil {
		in.c.OnCursorChange(math.NaN())
	}
}

// cursorStep moves the cursor by a twentieth of the view; cursorEnd takes
// it to either end. Without a cursor the first key starts in the middle.
func (in *input) cursorStep(dir int) {
	vx := in.xView()
	x := in.hover.Get()
	if math.IsNaN(x) {
		x = vx.Lo + vx.Span()/2
	}
	in.setCursor(min(max(x+float64(dir)*vx.Span()/20, vx.Lo), vx.Hi))
}

func (in *input) cursorEnd(first bool) {
	vx := in.xView()
	if first {
		in.setCursor(vx.Lo)
	} else {
		in.setCursor(vx.Hi)
	}
}

// keyBindings binds the chart's keys while it has the focus, as base/plot's
// keys do: Left and Right step the cursor, Home and End take it to the
// view's ends, Escape clears it.
func keyBindings(in *input, focused func() bool) core.Node {
	return core.Show{When: focused, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("left", core.KeyOptions{Repeat: true}, func() { in.cursorStep(-1) })
		cx.OnKeyWith("right", core.KeyOptions{Repeat: true}, func() { in.cursorStep(1) })
		cx.OnKey("home", func() { in.cursorEnd(true) })
		cx.OnKey("end", func() { in.cursorEnd(false) })
		cx.OnKey("escape", in.clearCursor)
		return nil
	}}
}

func (in *input) onMouseLeave(core.MouseEvent) {
	if in.drag.active {
		return
	}
	in.clearCursor()
}

func (in *input) onPressDown(e core.PressEvent) {
	x := in.xAt(float64(e.Position.X))
	in.drag = dragState{active: true, pan: e.Modifiers.Shift, fromX: x, toX: x, view: in.ownView()}
	if in.c.Measure && !in.drag.pan {
		fx := in.frac(x)
		switch {
		case math.Abs(fx-in.markA.Get()) < 0.02:
			in.drag.marker = 1
		case math.Abs(fx-in.markB.Get()) < 0.02:
			in.drag.marker = 2
		}
	}
}

func (in *input) onPressUp(e core.PressEvent) {
	if !in.drag.active {
		return
	}
	d := in.drag
	in.drag = dragState{}
	if d.pan || d.marker != 0 {
		return
	}
	x := in.xAt(float64(e.Position.X))
	lo, hi := min(d.fromX, x), max(d.fromX, x)
	if hi-lo > in.slop() {
		in.setView(Viewport{X: Range{lo, hi}})
	}
}

// onPress fits the view on a double click.
func (in *input) onPress(e core.PressEvent) {
	if e.Clicks >= 2 {
		in.setView(Viewport{})
		in.yZoom.Set(1)
	}
}

func (in *input) onWheel(e core.WheelEvent) {
	if e.Modifiers.Alt {
		z := in.yZoom.Get()
		in.yZoom.Set(min(max(z*(1-float64(e.Delta.Y)/400), 0.05), 20))
		return
	}
	if !e.Modifiers.Control && !e.Modifiers.Platform {
		// A wheel without a modifier goes on to the scroll views around
		// the chart, as it does in mygogo's plot.
		return
	}
	step := float64(e.Delta.Y)
	if e.Touchpad {
		step /= 4
	}
	at := in.xAt(float64(e.Position.X))
	in.setView(in.ownView().ZoomX(math.Pow(1.25, step/100), at))
}

func (in *input) onPinch(e core.GestureEvent) {
	if e.Value <= 0 {
		return
	}
	at := in.xAt(float64(e.Position.X))
	in.setView(in.ownView().ZoomX(1/float64(e.Value), at))
}

// onAccessibilityAction steps the cursor through the visible range, as a
// screen reader's increment and decrement do.
func (in *input) onAccessibilityAction(name string) {
	vx := in.xView()
	x := in.hover.Get()
	if math.IsNaN(x) {
		x = vx.Lo + vx.Span()/2
	}
	step := vx.Span() / 20
	switch name {
	case "increment":
		x += step
	case "decrement":
		x -= step
	default:
		return
	}
	x = min(max(x, vx.Lo), vx.Hi)
	in.setCursor(x)
}
