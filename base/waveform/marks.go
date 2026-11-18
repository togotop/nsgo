package waveform

import (
	"math"
	"strconv"

	"github.com/nsgo-dev/nsgo/core"
)

// parts holds the signals a Chart's interactive layers read. The drawing
// itself is in a canvas; everything that follows the pointer or a marker
// is a view, so moving it never records the waveform again.
type parts struct {
	c            Chart
	f            *core.Memo[*frame]
	hover        *core.Signal[float64]
	fade         *core.AnimatedValue
	markA, markB *core.Signal[float64]
	states       []*seriesState
	plotSize     *core.Signal[core.Size]
}

// markSource returns the channel Value-mode anchors read from, and the
// frame whose y ranges turn its values into screen positions. A missing
// frame or channel yields nil and the caller keeps the markers at rest.
func (p parts) markSource() (*seriesState, *frame) {
	f := p.f.Get()
	if f == nil || len(p.states) == 0 {
		return nil, nil
	}
	i := p.c.MarkChannel
	if i < 0 || i >= len(p.states) {
		i = 0
	}
	return p.states[i], f
}

// markValue is the selected channel's value at an anchor's screen-x
// fraction, the waveform point a vertical line crosses.
func (p parts) markValue(xFrac float64) (float64, bool) {
	st, f := p.markSource()
	if st == nil || f == nil || st.d == nil {
		return 0, false
	}
	v, ok := st.d.valueAt(p.c.xAtFrac(xFrac, f.vx))
	return float64(v), ok
}

// markYFrac turns an anchor's value into a fraction down the plot from the
// top, the screen position of the horizontal line through that waveform
// point. It falls back to the fraction itself when there is no range.
func (p parts) markYFrac(xFrac float64) float64 {
	st, f := p.markSource()
	if st == nil || f == nil || st.axis < 0 || st.axis >= len(f.ranges) {
		return xFrac
	}
	v, ok := p.markValue(xFrac)
	if !ok {
		return xFrac
	}
	return yFrac(v, f.ranges[st.axis], f.logs[st.axis])
}

// cursorX is the data x under this chart's pointer, or the group's cursor
// when another chart of the group owns it.
func (p parts) cursorX() float64 {
	x := p.hover.Get()
	if !math.IsNaN(x) {
		return x
	}
	return p.c.Sync.cursorOf()
}

func (p parts) frac(x float64) core.Length {
	f := p.f.Get()
	if f == nil || f.vx.Span() <= 0 {
		return core.Pct(0)
	}
	// xFrac keeps the cross line, tooltip and notes on the pointer on a
	// log x axis, where a linear fraction would put them elsewhere.
	return core.Pct(float32(p.c.xFrac(x, f.vx) * 100))
}

// valueText is a series' reading at x, or its newest sample without a
// cursor, formatted with its axis' words.
func (p parts) valueText(st *seriesState, x float64) string {
	if st == nil || st.d == nil || len(st.d.y) == 0 {
		return ""
	}
	if math.IsNaN(x) {
		x = st.d.extent().Hi
	}
	v, ok := st.d.valueAt(x)
	if !ok {
		return ""
	}
	return formatValue(float64(v), p.c.axisAt(st.axis).Format)
}

func (p parts) nodes() []core.Node {
	var out []core.Node
	if p.c.Cursor {
		out = append(out, core.Show{
			When: func() bool { return !math.IsNaN(p.cursorX()) },
			Then: func(*core.Scope) core.Node {
				return core.View{
					TestID: derivedID(p.c.TestID, "-cross"),
					Class:  p.c.CrossLineClass,
					Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0),
						Width: core.Pt(1), Animated: &core.AnimatedStyle{Opacity: p.fade}}.Merge(p.c.CrossLineStyle),
					StyleFunc:     func() core.Style { return core.Style{Left: p.frac(p.cursorX())} },
					PointerEvents: core.PointerEventsNone,
				}
			},
		})
		if p.c.Tooltip != nil {
			out = append(out, core.Show{
				When: func() bool { return !math.IsNaN(p.cursorX()) },
				Then: func(*core.Scope) core.Node {
					return core.View{
						TestID: derivedID(p.c.TestID, "-tipbox"),
						Style: core.Style{Position: core.Absolute, Top: core.Pt(0),
							Animated: &core.AnimatedStyle{Opacity: p.fade}},
						PointerEvents: core.PointerEventsNone,
						StyleFunc: func() core.Style {
							return core.Style{Left: p.frac(p.cursorX()),
								Margin: core.Edges{Left: core.Points(p.c.TooltipOffset)}}
						},
						Children: []core.Node{p.c.Tooltip},
					}
				},
			})
		}
	}
	if p.c.Legend {
		out = append(out, p.legend())
	}
	if p.c.Measure {
		out = append(out, p.markers()...)
	}
	out = append(out, p.notes()...)
	out = append(out, p.bandLabels()...)
	out = append(out, p.readings()...)
	return out
}

func (p parts) legend() core.Node {
	items := make([]core.Node, 0, len(p.states))
	for i, st := range p.states {
		idx := i
		if st.cfg.Hidden || st.cfg.Name == "" {
			continue
		}
		items = append(items, core.View{
			Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), Background: st.cfg.Color}},
				core.Text{Content: st.cfg.Name},
				core.Text{ContentFunc: func() string { return p.valueText(p.states[idx], p.cursorX()) }},
			},
		})
	}
	return core.View{
		Class: p.c.LegendClass,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(2),
			Right: core.Pt(rightRoom), Direction: core.Row, Gap: 16}.Merge(p.c.LegendStyle),
		Children: items,
	}
}

// markers draws the two A/B anchors. In Time mode they are vertical lines
// at the anchors' x; in Value mode they are horizontal lines through each
// anchor's value on the selected channel. Both modes share the same anchors,
// so switching orientation keeps the markers on the same waveform points.
func (p parts) markers() []core.Node {
	if p.c.MarkReading == MarkReadingNone {
		return nil
	}
	valueMode := p.c.MarkReading == MarkReadingValue
	out := make([]core.Node, 0, 4)
	for i, mark := range []*core.Signal[float64]{p.markA, p.markB} {
		m := mark
		id := ""
		if p.c.TestID != "" {
			id = p.c.TestID + []string{"-mark-a", "-mark-b"}[i]
		}
		if valueMode {
			// A horizontal line spans the plot at the anchor's value height.
			out = append(out, core.View{
				TestID: id,
				Class:  p.c.MarkClass,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0),
					Height: core.Pt(1)}.Merge(p.c.MarkStyle),
				StyleFunc: func() core.Style {
					return core.Style{Top: core.Pct(float32(p.markYFrac(m.Get()) * 100))}
				},
			})
		} else {
			out = append(out, core.View{
				TestID: id,
				Class:  p.c.MarkClass,
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0),
					Width: core.Pt(1)}.Merge(p.c.MarkStyle),
				StyleFunc: func() core.Style {
					return core.Style{Left: core.Pct(float32(m.Get() * 100))}
				},
			})
		}
		if label := p.c.MarkLabels[i]; label != "" {
			out = append(out, core.Text{
				Content: label,
				Class:   p.c.MarkClass,
				StyleFunc: func() core.Style {
					if valueMode {
						return core.Style{Position: core.Absolute, Left: core.Pt(2),
							Top: core.Pct(float32(p.markYFrac(m.Get()) * 100)),
							Margin: core.Edges{Top: -12}}
					}
					return core.Style{Position: core.Absolute,
						Left: core.Pct(float32(m.Get() * 100)), Top: core.Pt(2),
						Margin: core.Edges{Left: 3}}
				},
			})
		}
	}
	return out
}

// readings shows each automatic measurement, its label and value, in a
// column over the plot's top right. A reading updates as the window moves,
// so a following chart's numbers keep pace with what it shows.
func (p parts) readings() []core.Node {
	if len(p.c.Measurements) == 0 {
		return nil
	}
	items := make([]core.Node, 0, len(p.c.Measurements))
	for i, m := range p.c.Measurements {
		mm := m
		items = append(items, core.Text{
			TestID:      derivedID(p.c.TestID, "-m"+strconv.Itoa(i)),
			ContentFunc: func() string { return p.readingText(mm) },
		})
	}
	return []core.Node{core.View{
		TestID:        derivedID(p.c.TestID, "-readings"),
		Class:         p.c.MeasurementClass,
		Style:         core.Style{Position: core.Absolute, Right: core.Pt(4), Top: core.Pt(2), Gap: 2}.Merge(p.c.MeasurementStyle),
		PointerEvents: core.PointerEventsNone,
		Children:      items,
	}}
}

// readingText is one measurement's label and value, or empty when the
// window has too little of the series to read it.
func (p parts) readingText(m Measurement) string {
	f := p.f.Get()
	if f == nil {
		return ""
	}
	if m.Kind == MeasureMarkDelta {
		// The value difference between the two anchors on the selected
		// channel. Over is deliberately MeasureView so the x-interval path
		// below is never taken; read the anchors directly instead.
		fa, fb := p.markA.Get(), p.markB.Get()
		if math.IsNaN(fa) || math.IsNaN(fb) {
			return ""
		}
		va, oka := p.markValue(fa)
		vb, okb := p.markValue(fb)
		if !oka || !okb {
			return ""
		}
		st, _ := p.markSource()
		var format func(float64) string
		if st != nil {
			format = p.c.axisAt(st.axis).Format
		}
		value := formatValue(math.Abs(vb-va), format)
		if m.Label == "" {
			return value
		}
		return m.Label + " " + value
	}
	vx := f.vx
	if m.Over == MeasureMarkers {
		a, b := p.markA.Get(), p.markB.Get()
		if math.IsNaN(a) || math.IsNaN(b) {
			return ""
		}
		x1, x2 := p.c.xAtFrac(a, f.vx), p.c.xAtFrac(b, f.vx)
		vx = Range{min(x1, x2), max(x1, x2)}
	}
	if m.Source < 0 || m.Source >= len(f.series) {
		return ""
	}
	// Read from the very points the line draws, already reduced to the
	// extremes of each pixel column. Widening the retained history then
	// costs a reading nothing, and a peak a column covers is never lost.
	pts := f.series[m.Source].pts
	xs := make([]float64, 0, len(pts)/2)
	ys := make([]float64, 0, len(pts)/2)
	for i := 0; i+1 < len(pts); i += 2 {
		col, v := float64(pts[i]), float64(pts[i+1])
		if col < 0 || col > float64(f.px) || v != v {
			continue
		}
		x := p.c.xAtFrac(col/float64(f.px), f.vx)
		if x < vx.Lo || x > vx.Hi {
			continue
		}
		xs = append(xs, x)
		ys = append(ys, v)
	}
	v, ok := measureFrom(m, xs, ys, vx)
	if !ok {
		return ""
	}
	var value string
	if m.Format != nil {
		value = m.Format(v)
	} else {
		var format func(float64) string
		if m.Source >= 0 && m.Source < len(p.states) {
			format = p.c.axisAt(p.states[m.Source].axis).Format
		}
		value = formatValue(v, format)
	}
	if m.Label == "" {
		return value
	}
	return m.Label + " " + value
}

func (p parts) notes() []core.Node {
	out := make([]core.Node, 0, len(p.c.Notes))
	for _, note := range p.c.Notes {
		n := note
		out = append(out, core.Text{
			Content: n.Text,
			StyleFunc: func() core.Style {
				f := p.f.Get()
				if f == nil {
					return core.Style{}
				}
				a := n.Axis
				if a < 0 || a >= len(f.ranges) {
					a = 0
				}
				y := yOf(float32(n.Y), f.ranges[a], float32(p.plotSize.Get().Height), f.logs[a])
				if n.X < f.vx.Lo || n.X > f.vx.Hi {
					return core.Style{Position: core.Absolute, Display: core.DisplayNone}
				}
				return core.Style{Position: core.Absolute, Left: p.frac(n.X),
					Top: core.Pt(core.Points(y)), Margin: core.Edges{Left: 6, Top: -8}}
			},
		})
	}
	return out
}

func (p parts) bandLabels() []core.Node {
	out := make([]core.Node, 0, len(p.c.Bands))
	for _, band := range p.c.Bands {
		if band.Label == "" {
			continue
		}
		b := band
		out = append(out, core.Text{
			Content: b.Label,
			StyleFunc: func() core.Style {
				f := p.f.Get()
				if f == nil {
					return core.Style{}
				}
				a := b.Axis
				if a < 0 || a >= len(f.ranges) {
					a = 0
				}
				y := yOf(float32(b.Hi), f.ranges[a], float32(p.plotSize.Get().Height), f.logs[a])
				return core.Style{Position: core.Absolute, Left: core.Pt(4), Top: core.Pt(core.Points(y)),
					Margin: core.Edges{Top: 2}}
			},
		})
	}
	return out
}
