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
	View    Viewport
	Sync    *Group

	Cursor bool
	Legend bool
	Measure bool
	Label  string
	Note   string

	Class          string
	Style          core.Style
	GridClass      string
	GridStyle      core.Style
	GridColor      core.Color
	LegendClass    string
	LegendStyle    core.Style
	AxisClass      string
	AxisStyle      core.Style
	CrossLineClass string
	CrossLineStyle core.Style
	MarkClass      string
	MarkStyle      core.Style
	Tooltip        core.Node
	TooltipOffset  core.Points

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
}

type frameSeries struct {
	pts  []float32
	cfg  Series
	axis int
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
	for i := range c.Series {
		if s := c.Series[i].Source; s != nil {
			s.attach(cx)
			if c.Follow > 0 {
				s.Retain(2 * c.Follow)
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

	xView := func() Range {
		e := extentOf(states)
		v := c.Sync.viewOf(view.Get())
		if c.Follow > 0 && e.Valid() {
			return Range{e.Hi - c.Follow, e.Hi}
		}
		if !v.X.Valid() {
			v = v.Fit(e)
		}
		return v.Clamp(e).X
	}
	setView := func(v Viewport) {
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
	in := newInput(c, view, hover, markA, markB, yZoom, fade, plotSize, xView, setView)

	decimated := core.NewMemo(cx, func() *frame {
		for _, st := range states {
			if st.cfg.Source != nil && st.cfg.Source.changed != nil {
				st.cfg.Source.changed.Get()
			}
		}
		vx := xView()
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
		for _, st := range states {
			if st.cfg.Hidden || st.d == nil {
				continue
			}
			sc := st.d.scanFor(vx, px)
			key := geomKey{lo: vx.Lo, hi: vx.Hi, px: px, version: st.d.version,
				n: len(st.d.y), x0: st.d.x0, dx: st.d.dx}
			pts := st.g.get(key, sc, vx, px)
			f.series = append(f.series, frameSeries{pts: pts, cfg: st.cfg, axis: st.axis})
			for i := 0; i+1 < len(pts); i += 2 {
				if pts[i] < 0 || pts[i] > float32(px) {
					continue
				}
				v := float64(pts[i+1])
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
		if c.Time {
			f.ticksX = timeTicks(vx.Lo, vx.Hi, 6, c.XFormat)
		} else {
			f.ticksX = linearTicks(vx.Lo, vx.Hi, 6, c.XFormat)
		}
		return f
	})

	draw := func(d *core.Drawing, s core.Size) {
		f := decimated.Get()
		if f == nil {
			return
		}
		w, h := float32(s.Width), float32(s.Height)
		d.SetStrokeColor(c.gridPaint()).SetLineWidth(1)
		for _, t := range f.ticksY[0] {
			y := h - float32(t.At)*h
			d.BeginPath().MoveTo(0, y).LineTo(w, y).Stroke()
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
		for _, fs := range f.series {
			pts := fs.pts
			if len(pts) < 2 {
				continue
			}
			r, log := f.ranges[fs.axis], f.logs[fs.axis]
			if fs.cfg.Fill != 0 {
				d.BeginPath().MoveTo(pts[0], h)
				for i := 0; i+1 < len(pts); i += 2 {
					d.LineTo(pts[i], yOf(pts[i+1], r, h, log))
				}
				d.LineTo(pts[len(pts)-2], h).ClosePath().SetFillColor(fs.cfg.Fill).Fill()
			}
			d.BeginPath().MoveTo(pts[0], yOf(pts[1], r, h, log))
			for i := 2; i+1 < len(pts); i += 2 {
				d.LineTo(pts[i], yOf(pts[i+1], r, h, log))
			}
			width := float32(fs.cfg.Width)
			if width <= 0 {
				width = 1.5
			}
			if fs.cfg.Color != 0 {
				d.SetStrokeColor(fs.cfg.Color).SetLineWidth(width).Stroke()
			} else {
				d.StrokeFromView(core.PaintCurrentColor).SetLineWidth(width).Stroke()
			}
		}
		for _, n := range c.Notes {
			col := (n.X - f.vx.Lo) / f.vx.Span() * float64(f.px)
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

	canvas := core.Canvas{
		Class: c.GridClass,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0),
			Right: core.Pt(0), Bottom: core.Pt(0)},
		OnLayout: func(b core.Bounds) { plotSize.Set(b.Size) },
		Draw:     draw,
	}

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
		states: states, plotSize: plotSize, left: left}
	children := []core.Node{canvas}
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
	root := core.View{
		Class:        c.Class,
		Style:        core.Style{Height: core.Pt(height)}.Merge(c.Style),
		Accessible:   true,
		Role:         core.RoleImage,
		Label:        c.Label,
		TestID:       c.TestID,
		OnMouseMove:  in.onMouseMove,
		OnMouseLeave: in.onMouseLeave,
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
