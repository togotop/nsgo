package waveform

import (
	"fmt"
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	defaultHeight core.Points = 200
	axisRoom      core.Points = 52
	bottomRoom    core.Points = 22
	topRoom       core.Points = 8
	rightRoom     core.Points = 8

	// retainFactor is how many windows of history a source keeps. History
	// already trimmed cannot come back, so a zoom-out needs a reserve to
	// fill its wider view at once rather than waiting for new samples.
	retainFactor = 4
)

// Chart is a waveform view: its series, what it shows and how it draws.
// Its root is a view the app styles; the words it shows (axis labels,
// legend, notes) come from the app's Format, Label, Note and Title, so the
// component holds no strings of its own.
//
// Follow pins the view to the newest samples, a stretch of Follow x units
// wide. Time says x is Unix seconds, and the x axis picks round time
// intervals; its labels still go through the app's formatter.
type Chart struct {
	Series []Series
	Axes   []Axis
	Bands  []Band
	Notes  []Note
	Height core.Points
	Time   bool
	// XFormat labels the x axis' ticks. Without it a number formats
	// compactly; a Time chart's app gives the words for a date or a time.
	XFormat func(float64) string
	Follow  float64
	// TimeBase is x units per horizontal division; with Divisions it sets
	// the window a following or triggered chart shows. Zero uses Follow.
	TimeBase float64
	// DivisionsX and DivisionsY are the scope grid's columns and rows. Zero
	// keeps the axis ticks for horizontal lines and draws no columns.
	DivisionsX, DivisionsY int
	// XLog draws x on a logarithmic scale, for frequencies and other
	// quantities that span decades. The view's low end must be positive.
	XLog bool
	// View is where the chart starts. A zoom or a pan the user makes
	// stays in the component: an app hears it with OnViewChange but
	// cannot set it back into a running chart.
	View Viewport
	Sync *Group

	// Trigger pins an edge in the view, and Run stops, starts or takes one
	// capture. Both are optional; without them a chart follows its data.
	Trigger *Trigger
	Run     *Run

	Cursor  bool
	Legend  bool
	Measure bool
	// Snap makes the cursor and the measurement markers settle on a nearby
	// peak, trough or end of a series, within a few pixels.
	Snap bool
	// Measurements are automatic readings shown over the plot, apart from
	// the two markers Measure turns on.
	Measurements     []Measurement
	MeasurementClass string
	MeasurementStyle core.Style
	Label            string
	Note             string

	Class          string
	Style          core.Style
	GridClass      string
	GridColor      core.Color
	LegendClass    string
	LegendStyle    core.Style
	AxisClass      string
	AxisStyle      core.Style
	CrossLineClass string
	CrossLineStyle core.Style
	MarkClass      string
	MarkStyle      core.Style
	// MarkLabels names the two markers, as "A" and "B". Empty shows none;
	// the component holds no words of its own.
	MarkLabels    [2]string
	TriggerColor  core.Color
	Tooltip       core.Node
	TooltipOffset core.Points

	OnCursorChange func(float64)
	OnViewChange   func(Viewport)
	TestID         string
	OnLayout       func(core.Bounds)
}

func (c Chart) axisCount() int {
	if n := len(c.Axes); n > 0 {
		return n
	}
	return 1
}

// plotInsets is the room the y axes take beside the plot. The first axis
// sits on the left; every other one gets a column of its own on the right.
func (c Chart) plotInsets() (left, right core.Points) {
	left = axisRoom
	right = rightRoom
	if n := c.axisCount(); n > 1 {
		right += core.Points(n-1) * axisRoom
	}
	return
}

// baseWindow is the window a chart starts with, before the user zooms:
// its time base over the grid's columns, else Follow, else zero.
func (c Chart) baseWindow() float64 {
	if c.TimeBase > 0 {
		cols := c.DivisionsX
		if cols <= 0 {
			cols = 10
		}
		return c.TimeBase * float64(cols)
	}
	return c.Follow
}

// windowWidth is how much x a following or triggered chart shows: its base
// window, else the view's span, else the data's.
func (c Chart) windowWidth(v Viewport, e Range) float64 {
	if w := c.baseWindow(); w > 0 {
		return w
	}
	if v.X.Valid() {
		return v.X.Span()
	}
	return e.Span()
}

// xFrac is where x lies across vx, 0 to 1, on a log x axis too.
func (c Chart) xFrac(x float64, vx Range) float64 {
	if vx.Span() <= 0 {
		return 0
	}
	if c.XLog && vx.Lo > 0 && x > 0 {
		llo, lhi := math.Log10(vx.Lo), math.Log10(vx.Hi)
		if lhi > llo {
			return (math.Log10(x) - llo) / (lhi - llo)
		}
	}
	return (x - vx.Lo) / vx.Span()
}

// xAtFrac is the data x at f across vx, 0 to 1, on a log x axis too.
func (c Chart) xAtFrac(f float64, vx Range) float64 {
	if c.XLog && vx.Lo > 0 {
		llo, lhi := math.Log10(vx.Lo), math.Log10(vx.Hi)
		if lhi > llo {
			return math.Pow(10, llo+f*(lhi-llo))
		}
	}
	return vx.Lo + f*vx.Span()
}

func (c Chart) axisAt(k int) Axis {
	if len(c.Axes) == 0 || k < 0 || k >= len(c.Axes) {
		return Axis{}
	}
	return c.Axes[k]
}

// seriesState is a series' samples and its reusable decimation, held for
// the chart's lifetime. A static series owns its data; a live one reads the
// Source's, which only the engine touches.
type seriesState struct {
	cfg    Series
	d      *data
	static data
	g      geom
	axis   int
	// live is the source's data while a stopped chart draws a copy of it.
	live *data
}

func (c Chart) states() []*seriesState {
	out := make([]*seriesState, len(c.Series))
	for i, s := range c.Series {
		st := &seriesState{cfg: s, axis: s.Axis}
		if st.axis < 0 || st.axis >= c.axisCount() {
			st.axis = 0
		}
		if s.Source != nil {
			st.d = &s.Source.d
		} else {
			st.static = data{x0: s.X0, dx: s.Dx, xs: s.X}
			if st.static.dx <= 0 {
				st.static.dx = 1
			}
			y := s.Y
			if s.Head != 0 && len(y) > 0 {
				rotated := make([]float32, len(y))
				for j := range rotated {
					rotated[j] = y[(s.Head+j)%len(y)]
				}
				y = rotated
			}
			st.static.append(y)
			st.d = &st.static
		}
		out[i] = st
	}
	return out
}

// frame is what one commit draws: the x view, the fitted y ranges, the
// ticks for the labels and each series' reduced points. It is built once
// per commit and read by the canvas and the axis labels alike.
type frame struct {
	vx     Range
	px     int
	ranges []Range
	logs   []bool
	ticksX []Tick
	ticksY [][]Tick
	series []frameSeries

	// trigger is where the trigger's edge sits across the view, 0 to 1,
	// when triggered says an edge was found.
	trigger   float64
	triggered bool
}

type frameSeries struct {
	pts  []float32
	cfg  Series
	axis int

	// base, scale and offset are the channel's vertical controls for this
	// frame, so the line and the fit read the same transformed value.
	base, scale, offset float64
}

// y is a sample after the channel's vertical controls.
func (fs frameSeries) y(v float32) float32 {
	return float32((float64(v)-fs.base)*fs.scale + fs.offset)
}

func extentOf(states []*seriesState) Range {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, st := range states {
		if st.cfg.Hidden || st.d == nil || len(st.d.y) == 0 {
			continue
		}
		e := st.d.extent()
		lo, hi = min(lo, e.Lo), max(hi, e.Hi)
	}
	if lo > hi {
		return Range{}
	}
	return Range{lo, hi}
}

// yOf maps a value onto the plot, from its bottom up.
func yOf(v float32, r Range, h float32, log bool) float32 {
	if log {
		x := float64(v)
		if x <= 0 {
			x = r.Lo
		}
		den := math.Log10(r.Hi) - math.Log10(r.Lo)
		if den <= 0 {
			return h
		}
		return h - float32((math.Log10(x)-math.Log10(r.Lo))/den)*h
	}
	if r.Span() <= 0 {
		return h
	}
	return h - float32((float64(v)-r.Lo)/r.Span())*h
}

// gridPaint is the color of the grid: the app's stroke, else a faint line.
func (c Chart) gridPaint() core.Color {
	if c.GridColor != 0 {
		return c.GridColor
	}
	return core.RGBA(0x888888, 0.35)
}

// Render implements core.Node.
func (c Chart) Render(cx *core.Scope) core.Node {
	if c.Run != nil {
		c.Run.attach(cx)
	}
	for i := range c.Series {
		if s := c.Series[i].Source; s != nil {
			s.attach(cx)
			// Keep twice the base window, so a following or time-based
			// chart does not grow without bound. A zoom widens it below.
			if w := c.baseWindow(); w > 0 {
				s.Retain(retainFactor * w)
			}
		}
	}
	if c.Sync != nil {
		c.Sync.attach(cx)
	}
	states := c.states()
	plotSize := core.NewSignal(cx, core.Size{})
	view := core.NewSignal(cx, c.View)
	hover := core.NewSignal(cx, math.NaN())
	fade := core.NewAnimatedValue(cx, 0)
	markA := core.NewSignal(cx, math.NaN())
	markB := core.NewSignal(cx, math.NaN())
	yZoom := core.NewSignal(cx, float64(1))
	focused := core.NewSignal(cx, false)

	var heldX Range
	// viewAnchor is how far the right end of a view the user zoomed or
	// panned sits behind the newest sample. Keeping it fixed makes a
	// running chart scroll at a steady rate instead of breathing as
	// Retain trims the history.
	var viewAnchor float64
	var anchored bool
	// triggerX is the edge the view last anchored on, so the line on the
	// plot and the view always agree. frozenView holds a single-shot
	// capture in a signal, so it survives a re-run of the drawing.
	var triggerX float64
	var haveTrigger bool
	frozenView := core.NewSignal(cx, Range{})
	xView := func() Range {
		e := extentOf(states)
		v := c.Sync.viewOf(view.Get())
		if c.Trigger != nil && c.Trigger.Mode == TriggerSingle {
			if f := frozenView.GetUntracked(); f.Valid() {
				return f
			}
		}
		if anchored && v.X.Valid() && e.Valid() {
			return followView(v, e, viewAnchor)
		}
		if c.Trigger != nil && e.Valid() {
			pos := c.Trigger.Position
			if pos <= 0 {
				pos = 0.5
			}
			pos = min(max(pos, 0), 1)
			// A view the user zoomed or panned takes over, so a stopped or
			// triggered chart can still be inspected; a double click brings
			// the trigger back.
			if v.X.Valid() {
				return v.Clamp(e).X
			}
			width := c.windowWidth(v, e)
			// Leave the trigger's share of the window after the edge, so the
			// view is filled instead of ending at the newest sample.
			margin := (1 - pos) * width
			if tx, ok := c.triggerAtBefore(states, width, e.Hi-margin); ok {
				r := Range{tx - pos*width, tx + (1-pos)*width}
				clamped := Viewport{X: r}.Clamp(e).X
				heldX = clamped
				triggerX, haveTrigger = tx, true
				return clamped
			}
			if c.Trigger.Mode != TriggerAuto && heldX.Valid() {
				return heldX
			}
		}
		if c.TimeBase > 0 && e.Valid() {
			return Viewport{X: Range{e.Hi - c.windowWidth(v, e), e.Hi}}.Clamp(e).X
		}
		if c.Follow > 0 && e.Valid() {
			return Viewport{X: Range{e.Hi - c.Follow, e.Hi}}.Clamp(e).X
		}
		if !v.X.Valid() {
			v = v.Fit(e)
		}
		return v.Clamp(e).X
	}
	setView := func(v Viewport) {
		// The history a source keeps must cover the view the user chose,
		// or a zoom-out shows only a short stretch of waveform. Retain only
		// grows; a double click gives the surplus back.
		base := retainFactor * c.baseWindow()
		for _, st := range states {
			if s := st.cfg.Source; s != nil {
				if v.X.Valid() {
					s.Retain(retainFactor * v.X.Span())
				} else if base > 0 {
					s.resizeRetain(base)
				}
			}
		}
		if v.X.Valid() {
			if e := extentOf(states); e.Valid() {
				viewAnchor = e.Hi - v.X.Hi
				anchored = true
			}
		} else {
			anchored = false
			// A double click re-arms a single-shot trigger and puts the
			// live samples back under it.
			frozenView.Set(Range{})
			for _, st := range states {
				if st.live != nil {
					st.d = st.live
					st.live = nil
				}
			}
		}
		if c.Sync != nil {
			c.Sync.setView(v)
		} else {
			view.Set(v)
		}
		if c.OnViewChange != nil {
			c.OnViewChange(v)
		}
	}
	if c.Sync != nil && c.OnViewChange != nil {
		cx.Effect(func() { c.OnViewChange(c.Sync.viewOf(view.Get())) })
	}
	snap := func(x float64) float64 {
		return c.snapX(x, states, xView(), int(plotSize.Get().Width))
	}
	in := newInput(c, view, hover, markA, markB, yZoom, fade, plotSize, xView, setView, snap)

	// Single arms one capture: it waits for an edge that arrives after
	// the press, then stops, leaving that capture on screen. An edge
	// already in the buffer would stop it the instant it was pressed.
	var armedAt float64
	if c.Trigger != nil && c.Run != nil {
		cx.Effect(func() {
			if !c.Run.single.Get() {
				armedAt = 0
				return
			}
			for _, st := range states {
				if st.cfg.Source != nil && st.cfg.Source.changed != nil {
					st.cfg.Source.changed.Get()
				}
			}
			e := extentOf(states)
			if !e.Valid() {
				return
			}
			if armedAt == 0 {
				armedAt = e.Hi
				return
			}
			width := c.windowWidth(Viewport{}, e)
			pos := c.Trigger.Position
			if pos <= 0 {
				pos = 0.5
			}
			pos = min(max(pos, 0), 1)
			if tx, ok := c.triggerAtBefore(states, width, e.Hi-(1-pos)*width); ok && tx > armedAt {
				c.Run.single.Set(false)
				c.Run.running.Set(false)
				armedAt = 0
			}
		})
	}

	decimated := core.NewMemo(cx, func() *frame {
		vx := xView()
		// Capture the first usable view, whether or not an edge was found,
		// and hold it for a single-shot trigger.
		if c.Trigger != nil && c.Trigger.Mode == TriggerSingle && vx.Valid() &&
			!frozenView.GetUntracked().Valid() {
			frozenView.Set(vx)
		}
		running := true
		if c.Run != nil && c.Run.running != nil {
			running = c.Run.running.Get()
		}
		// A single-shot trigger freezes the samples it captured, so Retain
		// cannot trim the held view out from under it.
		if c.Trigger != nil && c.Trigger.Mode == TriggerSingle && frozenView.GetUntracked().Valid() {
			running = false
		}
		if running {
			for _, st := range states {
				if st.live != nil {
					st.d = st.live
					st.live = nil
				}
				if st.cfg.Source != nil && st.cfg.Source.changed != nil {
					st.cfg.Source.changed.Get()
				}
			}
		} else {
			// Stopped: draw a copy of the samples taken now, so the view
			// still zooms and pans over the capture while the source keeps
			// filling, and the readings hold still.
			for _, st := range states {
				if st.live == nil && st.d != nil {
					st.live = st.d
					st.d = st.d.clone()
				}
			}
		}
		px := int(plotSize.Get().Width)
		if px <= 0 || !vx.Valid() {
			return nil
		}
		n := c.axisCount()
		f := &frame{vx: vx, px: px, ranges: make([]Range, n), logs: make([]bool, n), ticksY: make([][]Tick, n)}
		lo, hi := make([]float64, n), make([]float64, n)
		for k := range lo {
			lo[k], hi[k] = math.Inf(1), math.Inf(-1)
		}
		// One entry per Chart.Series, so a series' canvas reads its own by
		// index; a hidden one keeps its place with no points.
		f.series = make([]frameSeries, len(states))
		for idx, st := range states {
			if st.cfg.Hidden || st.d == nil {
				continue
			}
			sc := st.d.scanFor(vx, px, c.XLog)
			key := geomKey{lo: vx.Lo, hi: vx.Hi, px: px, version: st.d.version,
				n: len(st.d.y), x0: st.d.x0, dx: st.d.dx, logX: c.XLog}
			pts := st.g.get(key, sc, vx, px)
			base := 0.0
			if st.cfg.AC {
				// Remove the direct current from the same reduced points
				// the line draws, so AC coupling stays O(width) too.
				sum, n := 0.0, 0
				for i := 1; i < len(pts); i += 2 {
					if v := float64(pts[i]); v == v {
						sum += v
						n++
					}
				}
				if n > 0 {
					base = sum / float64(n)
				}
			}
			fs := frameSeries{pts: pts, cfg: st.cfg, axis: st.axis,
				base: base, scale: st.cfg.scaleY(), offset: st.cfg.Offset}
			f.series[idx] = fs
			for i := 0; i+1 < len(pts); i += 2 {
				if pts[i] < 0 || pts[i] > float32(px) {
					continue
				}
				v := float64(fs.y(pts[i+1]))
				lo[st.axis], hi[st.axis] = min(lo[st.axis], v), max(hi[st.axis], v)
			}
		}
		for k := range f.ranges {
			a := c.axisAt(k)
			r := fitRange(a, lo[k], hi[k])
			if !a.Log {
				if z := yZoom.Get(); z != 1 {
					mid, half := (r.Lo+r.Hi)/2, r.Span()/2*z
					r = Range{mid - half, mid + half}
				}
			}
			f.ranges[k] = r
			f.logs[k] = a.Log
			f.ticksY[k] = a.ticks(r.Lo, r.Hi, 4)
		}
		// The line marks the edge the view actually anchored on, in the
		// view's own scale, so it stays put when the view is held or zoomed
		// and stays correct on a log x axis.
		if c.Trigger != nil && haveTrigger && vx.Span() > 0 {
			f.trigger = c.xFrac(triggerX, vx)
			f.triggered = true
		}
		switch {
		case c.XLog && vx.Lo > 0:
			f.ticksX = Axis{Log: true, Format: c.XFormat}.ticks(vx.Lo, vx.Hi, 6)
		case c.Time:
			f.ticksX = timeTicks(vx.Lo, vx.Hi, 6, c.XFormat)
		default:
			f.ticksX = linearTicks(vx.Lo, vx.Hi, 6, c.XFormat)
		}
		return f
	})

	// The plot's layers, bottom to top: one canvas for the grid and the
	// bands, one per series, and one for the notes. A series' canvas takes
	// its Class, Style and StyleFunc, so the app styles that line as any
	// other part, as base/plot's marks do.
	baseDraw := func(d *core.Drawing, s core.Size) {
		f := decimated.Get()
		if f == nil {
			return
		}
		w, h := float32(s.Width), float32(s.Height)
		d.SetStrokeColor(c.gridPaint()).SetLineWidth(1)
		if c.DivisionsX > 0 || c.DivisionsY > 0 {
			cols, rows := c.DivisionsX, c.DivisionsY
			if cols <= 0 {
				cols = 10
			}
			if rows <= 0 {
				rows = 8
			}
			for i := 1; i < cols; i++ {
				x := w * float32(i) / float32(cols)
				d.BeginPath().MoveTo(x, 0).LineTo(x, h).Stroke()
			}
			for j := 1; j < rows; j++ {
				y := h * float32(j) / float32(rows)
				d.BeginPath().MoveTo(0, y).LineTo(w, y).Stroke()
			}
		} else if len(f.ticksY) > 0 {
			for _, t := range f.ticksY[0] {
				y := h - float32(t.At)*h
				d.BeginPath().MoveTo(0, y).LineTo(w, y).Stroke()
			}
		}
		if c.Trigger != nil && f.triggered && f.trigger >= 0 && f.trigger <= 1 {
			col := c.TriggerColor
			if col == 0 {
				col = core.RGBA(0xcc3333, 0.8)
			}
			x := float32(f.trigger) * w
			d.SetStrokeColor(col).SetLineWidth(1)
			d.BeginPath().MoveTo(x, 0).LineTo(x, h).Stroke()
		}
		for _, b := range c.Bands {
			a := b.Axis
			if a < 0 || a >= len(f.ranges) {
				a = 0
			}
			top := yOf(float32(b.Hi), f.ranges[a], h, f.logs[a])
			bottom := yOf(float32(b.Lo), f.ranges[a], h, f.logs[a])
			color := b.Color
			if color == 0 {
				color = core.RGBA(0x888888, 0.14)
			}
			d.SetFillColor(color).FillRect(0, min(top, bottom), w, float32(math.Abs(float64(bottom-top))))
		}
	}

	seriesDraw := func(idx int) func(*core.Drawing, core.Size) {
		return func(d *core.Drawing, s core.Size) {
			f := decimated.Get()
			if f == nil || idx >= len(f.series) {
				return
			}
			fs := f.series[idx]
			pts := fs.pts
			if len(pts) < 2 {
				return
			}
			h := float32(s.Height)
			r, log := f.ranges[fs.axis], f.logs[fs.axis]
			if fs.cfg.Fill != 0 {
				d.BeginPath()
				segments(pts, func(seg []float32) {
					if len(seg) < 2 {
						return
					}
					d.MoveTo(seg[0], h)
					for i := 0; i+1 < len(seg); i += 2 {
						d.LineTo(seg[i], yOf(fs.y(seg[i+1]), r, h, log))
					}
					d.LineTo(seg[len(seg)-2], h).ClosePath()
				})
				d.SetFillColor(fs.cfg.Fill).Fill()
			}
			d.BeginPath()
			segments(pts, func(seg []float32) {
				if len(seg) < 2 {
					return
				}
				d.MoveTo(seg[0], yOf(fs.y(seg[1]), r, h, log))
				for i := 2; i+1 < len(seg); i += 2 {
					d.LineTo(seg[i], yOf(fs.y(seg[i+1]), r, h, log))
				}
			})
			paint := core.PaintCurrentColor
			if fs.cfg.Color != 0 {
				paint = core.PaintColor(fs.cfg.Color)
			}
			d.StrokeFromView(paint).LineWidthFromView().LineDashFromView()
			d.Stroke()
		}
	}

	notesDraw := func(d *core.Drawing, s core.Size) {
		f := decimated.Get()
		if f == nil {
			return
		}
		h := float32(s.Height)
		for _, n := range c.Notes {
			col := c.xFrac(n.X, f.vx) * float64(f.px)
			if col < 0 || col > float64(f.px) {
				continue
			}
			a := n.Axis
			if a < 0 || a >= len(f.ranges) {
				a = 0
			}
			y := yOf(float32(n.Y), f.ranges[a], h, f.logs[a])
			color := n.Color
			if color == 0 {
				color = core.RGB(0x4477aa)
			}
			d.SetFillColor(color).BeginPath().Arc(float32(col), y, 3, 0, 2*math.Pi, false).Fill()
		}
	}

	fillPlot := core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0),
		Right: core.Pt(0), Bottom: core.Pt(0)}
	canvases := []core.Node{core.Canvas{
		TestID:   derivedID(c.TestID, "-grid"),
		Class:    c.GridClass,
		Style:    fillPlot,
		OnLayout: func(b core.Bounds) { plotSize.Set(b.Size) },
		Draw:     baseDraw,
	}}
	for i := range c.Series {
		if c.Series[i].Hidden {
			continue
		}
		s := c.Series[i]
		// The line's width is the view's StrokeWidth, so a StyleFunc may
		// change it every commit; Width is its default.
		st := s.Style
		if st.StrokeWidth <= 0 {
			if s.Width > 0 {
				st.StrokeWidth = s.Width
			} else {
				st.StrokeWidth = 1.5
			}
		}
		canvases = append(canvases, core.Canvas{
			TestID:    derivedID(c.TestID, fmt.Sprintf("-s%d", i)),
			Class:     s.Class,
			Style:     fillPlot.Merge(st),
			StyleFunc: s.StyleFunc,
			Draw:      seriesDraw(i),
		})
	}
	canvases = append(canvases, core.Canvas{
		TestID: derivedID(c.TestID, "-notes"),
		Style:  fillPlot,
		Draw:   notesDraw,
	})
	if c.Measure {
		// The markers are fractions of the view, not data x: they hold
		// their place on screen as a following window slides, and a zoom
		// keeps them measuring the stretch the user sees.
		cx.Effect(func() {
			if !math.IsNaN(markA.GetUntracked()) {
				return
			}
			markA.Set(1.0 / 3)
			markB.Set(2.0 / 3)
		})
	}

	left, right := c.plotInsets()
	p := parts{c: c, f: decimated, hover: hover, fade: fade, markA: markA, markB: markB,
		states: states, plotSize: plotSize}
	children := canvases
	children = append(children, axisLabels(c, decimated, false, 0, left)...)
	for k := 1; k < c.axisCount(); k++ {
		children = append(children, axisLabels(c, decimated, false, k, left)...)
	}
	children = append(children, axisLabels(c, decimated, true, 0, left)...)
	children = append(children, p.nodes()...)
	children = append(children, keyBindings(in, focused.Get))

	plotTop := topRoom
	if c.Legend {
		plotTop += 20
	}
	plot := core.View{
		Style: core.Style{Position: core.Absolute, Left: core.Pt(left), Top: core.Pt(plotTop),
			Right: core.Pt(right), Bottom: core.Pt(bottomRoom)},
		Children: children,
	}
	height := c.Height
	if height <= 0 {
		height = defaultHeight
	}
	// Keep room for the plot itself between the insets, or a very short
	// chart would give it a negative height.
	if least := plotTop + bottomRoom + 16; height < least {
		height = least
	}
	root := core.View{
		Class:        c.Class,
		Style:        core.Style{Height: core.Pt(height)}.Merge(c.Style),
		Accessible:   true,
		Role:         core.RoleImage,
		Label:        c.Label,
		TestID:       c.TestID,
		OnMouseMove:  in.onMouseMove,
		OnMouseLeave: in.onMouseLeave,
		OnTouch:      in.onTouch,
		OnWheel:      in.onWheel,
		OnPinch:      in.onPinch,
		OnPressDown:  in.onPressDown,
		OnPressUp:    in.onPressUp,
		OnPress:      in.onPress,
		OnFocus:      func() { focused.Set(true) },
		OnBlur:       func() { focused.Set(false) },
		OnLayout: func(b core.Bounds) {
			if c.OnLayout != nil {
				c.OnLayout(b)
			}
		},
		Children: []core.Node{plot},
	}
	if c.Cursor || c.Measure {
		root.Focusable = func() bool { return true }
		root.AccessibilityActions = []core.AccessibilityAction{{Name: "increment"}, {Name: "decrement"}}
		root.OnAccessibilityAction = in.onAccessibilityAction
	}
	if c.Note != "" {
		// Note describes what the chart shows, as "15 s at 1 kHz": it is
		// the element's value, read by a screen reader with its label.
		root.AccessibilityValue = func() core.AccessibilityValue {
			return core.AccessibilityValue{Text: c.Note}
		}
	}
	return root
}

// derivedID is a test ID's part, such as its grid's canvas or a series',
// or empty without a chart ID.
func derivedID(base, suffix string) string {
	if base == "" {
		return ""
	}
	return base + suffix
}

// axisLabels builds the tick labels of one axis: the bottom x axis when x
// is true, else the y axis at axis, the first on the left and every other
// in a column of its own on the right. Each is a text the app styles; a
// change of a tick's position or label moves it without making a new view.
func axisLabels(c Chart, f *core.Memo[*frame], x bool, axis int, left core.Points) []core.Node {
	ticks := func() []Tick {
		fr := f.Get()
		if fr == nil {
			return nil
		}
		if x {
			return fr.ticksX
		}
		if axis >= 0 && axis < len(fr.ticksY) {
			return fr.ticksY[axis]
		}
		return nil
	}
	const slots = 12
	out := make([]core.Node, 0, slots+1)
	if !x {
		if title := c.axisAt(axis).Title; title != "" {
			id := ""
			if c.TestID != "" {
				id = fmt.Sprintf("%s-y%d-title", c.TestID, axis)
			}
			s := core.Style{Position: core.Absolute, Top: core.Pt(2)}
			if axis == 0 {
				s.Left = core.Pt(-left)
				s.Width = core.Pt(left - 8)
			} else {
				s.Left = core.Pct(100)
				s.Margin = core.Edges{Left: core.Points(axis-1)*axisRoom + 4}
			}
			out = append(out, core.Text{
				TestID:  id,
				Class:   c.AxisClass,
				Content: title,
				Style:   s.Merge(c.AxisStyle),
			})
		}
	}
	for i := range slots {
		id := ""
		if c.TestID != "" {
			if x {
				id = fmt.Sprintf("%s-x-%d", c.TestID, i)
			} else {
				id = fmt.Sprintf("%s-y%d-%d", c.TestID, axis, i)
			}
		}
		out = append(out, core.Show{When: func() bool { return i < len(ticks()) },
			Then: func(*core.Scope) core.Node {
				return core.Text{
					TestID: id,
					Class:  c.AxisClass,
					ContentFunc: func() string {
						ts := ticks()
						if i >= len(ts) {
							return ""
						}
						return ts[i].Label
					},
					StyleFunc: func() core.Style {
						ts := ticks()
						if i >= len(ts) {
							return core.Style{}
						}
						t := ts[i]
						s := core.Style{Position: core.Absolute}
						switch {
						case x:
							s.Left = core.Pct(float32(t.At * 100))
							s.Top = core.Pct(100)
							s.Margin = core.Edges{Top: 4}
						case axis == 0:
							s.Left = core.Pt(-left)
							s.Top = core.Pct(float32((1 - t.At) * 100))
							s.Width = core.Pt(left - 8)
							s.Margin = core.Edges{Top: -7}
						default:
							s.Left = core.Pct(100)
							s.Top = core.Pct(float32((1 - t.At) * 100))
							s.Margin = core.Edges{Left: core.Points(axis-1)*axisRoom + 4, Top: -7}
						}
						return s.Merge(c.AxisStyle)
					},
				}
			},
		})
	}
	return out
}
