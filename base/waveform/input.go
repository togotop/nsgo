package waveform

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// dragState is a press in progress: a drag pans, Shift and a drag selects
// a range to zoom to, Alt and a drag sets the markers' interval, and a
// press near a measurement marker moves it.
type dragState struct {
	active bool
	mouse  bool // the press is the mouse's, not a finger's echo of it
	zoom   bool
	sel    bool // Alt: a drag sets the markers' interval
	marker int  // 0: none, 1: A, 2: B
	fromX  float64
	view   Viewport
	moved  bool
}

// touchState is a finger drag in progress: one finger pans, two pinch the
// view, as a scope's touch surface does.
type touchState struct {
	active   bool
	pinch    bool
	lastX    float64
	lastDist float64
	view     Viewport
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
	snap     func(float64) float64
	frameOf  func() *frame // the drawn frame, for Value-mode anchor lookup
	drag     dragState
	touch    touchState
	pinched  bool
}

func newInput(c Chart, view *core.Signal[Viewport], hover, markA, markB, yZoom *core.Signal[float64],
	fade *core.AnimatedValue, plotSize *core.Signal[core.Size], xView func() Range, setView func(Viewport),
	snap func(float64) float64, frameOf func() *frame) *input {
	return &input{c: c, view: view, hover: hover, fade: fade, markA: markA, markB: markB,
		yZoom: yZoom, plotSize: plotSize, xView: xView, setView: setView, snap: snap, frameOf: frameOf}
}

// ownView is the view this chart shows, fitted and clamped to the data,
// as xView reports it: a zoom or a pan starts from what the user sees,
// not from the uninitialized viewport a Chart begins with.
func (in *input) ownView() Viewport { return Viewport{X: in.xView()} }

// xAt turns a pointer's x in the root view into a data x. The plot is
// inset by axisRoom on the left, and its width is what the canvas reports.
func (in *input) xAt(px float64) float64 { return in.xAtIn(px, in.xView()) }

// xAtIn turns a pointer's x in the root view into a data x in the view vx.
// A drag maps the pointer in the view it started from: mapping it in the
// view it is moving would feed each step's own pan back into the next
// step's reading, and the waveform would shake instead of sliding.
func (in *input) xAtIn(px float64, vx Range) float64 {
	w := float64(in.plotSize.Get().Width)
	if w <= 0 {
		return 0
	}
	f := min(max((px-float64(axisRoom))/w, 0), 1)
	return in.c.xAtFrac(f, vx)
}

// moved reports whether the pointer has travelled two percent of the view
// on screen, which is the same test on a linear and a log x axis.
func (in *input) moved(x, from float64) bool {
	vx := in.xView()
	if vx.Span() <= 0 {
		return false
	}
	return math.Abs(in.c.xFrac(x, vx)-in.c.xFrac(from, vx)) > 0.02
}

// frac is where a data x lies in the view, 0 to 1. Markers are kept as
// fractions so they hold their place on screen as the window moves.
func (in *input) frac(x float64) float64 {
	vx := in.xView()
	if vx.Span() <= 0 {
		return 0
	}
	return min(max(in.c.xFrac(x, vx), 0), 1)
}

// yFracAt turns a pointer's y in the root view into a fraction down the
// plot (0 = top). The canvas starts below topRoom, like x starts at
// axisRoom. It is the register a horizontal marker is dragged in.
func (in *input) yFracAt(py float64) float64 {
	h := float64(in.plotSize.Get().Height)
	if h <= 0 {
		return 0
	}
	return min(max((py-float64(topRoom))/h, 0), 1)
}

// anchorAtValue finds the x fraction on the selected channel whose value
// is closest to v, searching the frame the line actually draws and
// preferring the crossing nearest nearX. It returns false when the series
// has no samples in view, the "no定位点" case that keeps the old anchor.
func (in *input) anchorAtValue(v float64, nearX float64) (float64, bool) {
	if in.frameOf == nil {
		return 0, false
	}
	f := in.frameOf()
	if f == nil {
		return 0, false
	}
	i := in.c.MarkChannel
	if i < 0 || i >= len(f.series) {
		i = 0
	}
	fs := f.series[i]
	if len(fs.pts) < 2 {
		return 0, false
	}
	best, bestErr, found := 0.0, math.Inf(1), false
	for k := 0; k+1 < len(fs.pts); k += 2 {
		col, sample := float64(fs.pts[k]), float64(fs.pts[k+1])
		if col < 0 || col > float64(f.px) || sample != sample {
			continue
		}
		value := float64(fs.y(float32(sample)))
		err := math.Abs(value - v)
		if err < bestErr {
			bestErr, best, found = err, col/float64(f.px), true
		}
	}
	return best, found
}

// setAnchor writes a dragged position to a marker: x for vertical lines,
// or a value-derived x for horizontal ones.
func (in *input) setAnchor(mark *core.Signal[float64], py, x float64) {
	if in.c.MarkReading == MarkReadingValue {
		f := in.frameOf()
		if f == nil {
			return
		}
		i := in.c.MarkChannel
		if i < 0 || i >= len(f.ranges) {
			i = 0
		}
		v := yAtFrac(in.yFracAt(py), f.ranges[i], f.logs[i])
		if xf, ok := in.anchorAtValue(v, mark.Get()); ok {
			mark.Set(xf)
		}
		return
	}
	mark.Set(in.frac(in.snap(x)))
}

// markPos is a marker's screen position along the active axis: its x
// fraction in Time mode, or the value-height of its anchor in Value mode.
// Hit testing compares the pointer against this.
func (in *input) markPos(mark *core.Signal[float64]) float64 {
	if in.c.MarkReading != MarkReadingValue {
		return mark.Get()
	}
	f := in.frameOf()
	if f == nil {
		return mark.Get()
	}
	i := in.c.MarkChannel
	if i < 0 || i >= len(f.series) {
		i = 0
	}
	fs := f.series[i]
	if len(fs.pts) < 2 {
		return mark.Get()
	}
	x := in.c.xAtFrac(mark.Get(), f.vx)
	// The value on the drawn sample column nearest this anchor x.
	value, nearest, found := 0.0, math.Inf(1), false
	for k := 0; k+1 < len(fs.pts); k += 2 {
		col, sample := float64(fs.pts[k]), float64(fs.pts[k+1])
		if sample != sample || col < 0 || col > float64(f.px) {
			continue
		}
		cx := in.c.xAtFrac(col/float64(f.px), f.vx)
		if d := math.Abs(cx - x); d < nearest {
			nearest, value, found = d, float64(fs.y(float32(sample))), true
		}
	}
	if !found || i >= len(f.ranges) {
		return mark.Get()
	}
	return yFrac(value, f.ranges[i], f.logs[i])
}

// markHit reports which marker (1=A, 2=B) the pointer is on, or 0. The
// active axis decides whether x or y is compared and which size sets the
// finger-width threshold.
func (in *input) markHit(px, py, x float64) int {
	size := in.plotSize.Get()
	var at, a, b, span float64
	if in.c.MarkReading == MarkReadingValue {
		at, a, b, span = in.yFracAt(py), in.markPos(in.markA), in.markPos(in.markB), float64(size.Height)
	} else {
		at, a, b, span = in.frac(x), in.markA.Get(), in.markB.Get(), float64(size.Width)
	}
	hit := 0.03
	if span > 0 {
		hit = max(hit, 16/span)
	}
	switch {
	case math.Abs(at-a) < hit:
		return 1
	case math.Abs(at-b) < hit:
		return 2
	}
	return 0
}

func (in *input) onMouseMove(e core.MouseEvent) {
	px := float64(e.Position.X)
	py := float64(e.Position.Y)
	x := in.xAt(px)
	if in.drag.active && in.drag.view.X.Valid() {
		x = in.xAtIn(px, in.drag.view.X)
	}
	if in.drag.active {
		switch {
		case in.drag.sel:
			in.setAnchor(in.markB, py, x)
		case in.drag.marker == 1:
			in.setAnchor(in.markA, py, x)
		case in.drag.marker == 2:
			in.setAnchor(in.markB, py, x)
		case !in.drag.zoom:
			// A dead zone keeps a press that is not quite on a marker from
			// panning the waveform on its first pixel of jitter.
			if !in.drag.moved {
				if !in.moved(x, in.drag.fromX) {
					return
				}
				in.drag.moved = true
			}
			// A pan moves the view by whole pixels: a fraction of one
			// puts different samples in every column each frame, so the
			// decimated trace shimmers as it slides. Whole pixels keep
			// the line still between steps, which reads as a smooth drag.
			in.setView(in.drag.view.PanX(in.snappedPan(in.drag.fromX - x)))
		}
		return
	}
	if e.PressedButton != core.MouseButtonNone || (!in.c.Cursor && !in.c.Measure) {
		return
	}
	in.setCursor(in.snap(x))
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

// cursorStep moves the cursor by a twentieth of the view on screen;
// cursorEnd takes it to either end. Both use the view's scale, so a log x
// axis steps by equal distances rather than equal data x.
func (in *input) cursorStep(dir int) {
	vx := in.xView()
	x := in.hover.Get()
	if math.IsNaN(x) {
		x = in.c.xAtFrac(0.5, vx)
	}
	f := in.c.xFrac(x, vx)
	in.setCursor(in.c.xAtFrac(min(max(f+float64(dir)*0.05, 0), 1), vx))
}

func (in *input) cursorEnd(first bool) {
	vx := in.xView()
	if first {
		in.setCursor(in.c.xAtFrac(0, vx))
	} else {
		in.setCursor(in.c.xAtFrac(1, vx))
	}
}

// snappedPan rounds a drag's data distance to whole pixels of the view.
func (in *input) snappedPan(dx float64) float64 {
	if w := float64(in.plotSize.Get().Width); w > 0 {
		if span := in.drag.view.X.Span(); span > 0 {
			step := span / w
			return math.Round(dx/step) * step
		}
	}
	return dx
}

// panView slides the view by a tenth of what it shows, as mygogo's arrow
// keys do. The bare arrow keys belong to the cursor, so these take Shift.
func (in *input) panView(dir int) {
	vx := in.xView()
	if !vx.Valid() {
		return
	}
	in.setView(in.ownView().PanX(float64(dir) * vx.Span() * 0.1))
}

// zoomView scales the view around its middle from the keyboard.
func (in *input) zoomView(f float64) {
	vx := in.xView()
	if !vx.Valid() {
		return
	}
	in.setView(in.zoom(f, (vx.Lo+vx.Hi)/2))
}

// fitView resets the view, as a double click does.
func (in *input) fitView() {
	in.setView(Viewport{})
	in.yZoom.Set(1)
}

// keyBindings binds the chart's keys while it has the focus: Left and Right
// step the cursor, Home and End take it to the view's ends and Escape clears
// it; with Shift they pan and fit the view, and + and - zoom it, as mygogo's
// plot does with its bare arrows and +/- keys.
func keyBindings(in *input, focused func() bool) core.Node {
	return core.Show{When: focused, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("left", core.KeyOptions{Repeat: true}, func() { in.cursorStep(-1) })
		cx.OnKeyWith("right", core.KeyOptions{Repeat: true}, func() { in.cursorStep(1) })
		cx.OnKey("home", func() { in.cursorEnd(true) })
		cx.OnKey("end", func() { in.cursorEnd(false) })
		cx.OnKey("escape", in.clearCursor)
		cx.OnKeyWith("shift+left", core.KeyOptions{Repeat: true}, func() { in.panView(-1) })
		cx.OnKeyWith("shift+right", core.KeyOptions{Repeat: true}, func() { in.panView(1) })
		cx.OnKey("equal", func() { in.zoomView(0.8) })
		cx.OnKey("minus", func() { in.zoomView(1.25) })
		cx.OnKey("shift+home", in.fitView)
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
	px, py := float64(e.Position.X), float64(e.Position.Y)
	x := in.xAt(px)
	in.drag = dragState{active: true, mouse: true, zoom: e.Modifiers.Shift, sel: e.Modifiers.Alt, fromX: x, view: in.ownView()}
	if in.c.Measure {
		if in.drag.sel {
			in.setAnchor(in.markA, py, x)
			in.markB.Set(in.markA.Get())
			return
		}
		if !in.drag.zoom {
			// Give the markers a finger's width, about ten points, so a
			// drag meant for one does not pan the chart instead. The active
			// axis picks whether that width is measured in x or y.
			in.drag.marker = in.markHit(px, py, x)
		}
	}
}

func (in *input) onPressUp(e core.PressEvent) {
	if !in.drag.active {
		return
	}
	d := in.drag
	in.drag = dragState{}
	px, py := float64(e.Position.X), float64(e.Position.Y)
	x := in.xAt(px)
	if d.sel {
		if in.c.Measure {
			if in.c.MarkReading == MarkReadingValue {
				// Horizontal selection: A is where the press started, B is
				// where it ended; value order, not x order, decides which
				// line is higher, so keep them as pressed.
				in.setAnchor(in.markB, py, x)
			} else {
				a, b := in.markA.Get(), in.frac(in.snap(x))
				in.markA.Set(min(a, b))
				in.markB.Set(max(a, b))
			}
		}
		return
	}
	if d.marker != 0 || !d.zoom {
		return
	}
	lo, hi := min(d.fromX, x), max(d.fromX, x)
	if in.moved(x, d.fromX) {
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

// zoom scales the view around at, in log space on a log x axis so a decade
// keeps the width it had.
func (in *input) zoom(f, at float64) Viewport {
	v := in.ownView()
	if in.c.XLog && v.X.Lo > 0 && at > 0 {
		llo, lhi, lat := math.Log10(v.X.Lo), math.Log10(v.X.Hi), math.Log10(at)
		if lhi > llo {
			lo, hi := lat-(lat-llo)*f, lat+(lhi-lat)*f
			return Viewport{X: Range{math.Pow(10, lo), math.Pow(10, hi)}}
		}
	}
	return v.ZoomX(f, at)
}

// onTouch pans with one finger and pinches with two, as a touch surface
// does; a mouse pans through OnMouseMove instead.
//
// A desktop reports its mouse as a finger too, so components that only
// know touches work with one. A chart knows both, and while a mouse press
// is under way those touches are its echo: letting them pan as well would
// move the view twice, and clearing the drag on their end would leave the
// chart where the first small step took it.
func (in *input) onTouch(e core.TouchEvent) {
	if in.drag.active && in.drag.mouse && in.drag.marker == 0 {
		if e.Phase == core.TouchEnd || e.Phase == core.TouchCancel {
			in.touch = touchState{}
		}
		return
	}
	switch e.Phase {
	case core.TouchStart:
		if len(e.Touches) == 0 {
			return
		}
		in.drag = dragState{}
		// A finger on a marker moves it, as a mouse does, instead of
		// starting a pan that would move the waveform under it.
		if in.c.Measure && len(e.Touches) == 1 {
			t := e.Touches[0].Position
			hit := in.markHit(float64(t.X), float64(t.Y), in.xAt(float64(t.X)))
			if hit != 0 {
				in.drag = dragState{active: true, marker: hit}
				return
			}
		}
		in.touch = touchState{active: true, view: in.ownView()}
		in.touch.lastX = in.xAt(float64(e.Touches[0].Position.X))
		if len(e.Touches) >= 2 {
			in.touch.pinch = true
			in.touch.lastDist = math.Abs(float64(e.Touches[0].Position.X - e.Touches[1].Position.X))
		}
	case core.TouchMove:
		if in.drag.active && in.drag.marker != 0 && len(e.Touches) == 1 {
			t := e.Touches[0].Position
			mark := in.markB
			if in.drag.marker == 1 {
				mark = in.markA
			}
			in.setAnchor(mark, float64(t.Y), in.xAt(float64(t.X)))
			return
		}
		if !in.touch.active {
			return
		}
		if len(e.Touches) >= 2 {
			in.touch.pinch = true
			dist := math.Abs(float64(e.Touches[0].Position.X - e.Touches[1].Position.X))
			// A trackpad pinch arrives as a GestureEvent too. Let onPinch
			// own the scale when it does, so the two paths do not add up.
			if in.pinched {
				in.touch.lastDist = dist
				return
			}
			at := in.xAt(float64(e.Touches[0].Position.X+e.Touches[1].Position.X) / 2)
			if in.touch.lastDist > 0 && dist > 0 {
				in.setView(in.zoom(in.touch.lastDist/dist, at))
			}
			in.touch.lastDist = dist
			return
		}
		if len(e.Touches) == 1 {
			// The finger's x is read in the view the pan started from, as a
			// mouse drag's is: the moving view must not steer its own
			// reading, or the waveform shakes in place.
			px := float64(e.Touches[0].Position.X)
			x := in.xAt(px)
			if in.touch.view.X.Valid() {
				x = in.xAtIn(px, in.touch.view.X)
			}
			if in.touch.pinch {
				// One finger left the pinch: start a fresh pan here.
				in.touch.pinch = false
				in.touch.view = in.ownView()
				in.touch.lastX = x
				return
			}
			in.setView(in.touch.view.PanX(in.touch.lastX - x))
		}
	case core.TouchEnd, core.TouchCancel:
		in.touch = touchState{}
		in.drag = dragState{}
		in.pinched = false
	}
}

func (in *input) onWheel(e core.WheelEvent) {
	if e.Modifiers.Alt {
		z := in.yZoom.Get()
		in.yZoom.Set(min(max(z*(1-float64(e.Delta.Y)/400), 0.05), 20))
		return
	}
	if !e.Modifiers.Control && !e.Modifiers.Platform && !in.c.WheelZoom {
		// A wheel without a modifier goes on to the scroll views around
		// the chart, as it does in mygogo's plot, unless the app asked
		// for the wheel itself.
		return
	}
	step := float64(e.Delta.Y)
	if e.Touchpad {
		step /= 4
	}
	at := in.xAt(float64(e.Position.X))
	in.setView(in.zoom(math.Pow(1.25, step/100), at))
}

func (in *input) onPinch(e core.GestureEvent) {
	if e.Value <= 0 {
		return
	}
	in.pinched = true
	at := in.xAt(float64(e.Position.X))
	in.setView(in.zoom(1/float64(e.Value), at))
}

// onAccessibilityAction steps the cursor through the visible range, as a
// screen reader's increment and decrement do.
func (in *input) onAccessibilityAction(name string) {
	vx := in.xView()
	x := in.hover.Get()
	if math.IsNaN(x) {
		x = in.c.xAtFrac(0.5, vx)
	}
	f := in.c.xFrac(x, vx)
	switch name {
	case "increment":
		f += 0.05
	case "decrement":
		f -= 0.05
	default:
		return
	}
	in.setCursor(in.c.xAtFrac(min(max(f, 0), 1), vx))
}
