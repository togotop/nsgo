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
	return core.Pct(float32((x - f.vx.Lo) / f.vx.Span() * 100))
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

func (p parts) markers() []core.Node {
	out := make([]core.Node, 0, 3)
	for i, mark := range []*core.Signal[float64]{p.markA, p.markB} {
		m := mark
		id := ""
		if p.c.TestID != "" {
			id = p.c.TestID + []string{"-mark-a", "-mark-b"}[i]
		}
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
	out = append(out, core.Text{
		Class: p.c.MarkClass,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Bottom: core.Pt(2)},
		ContentFunc: func() string {
			fa, fb := p.markA.Get(), p.markB.Get()
			if math.IsNaN(fa) || math.IsNaN(fb) {
				return ""
			}
			f := p.f.Get()
			if f == nil {
				return ""
			}
			a := f.vx.Lo + fa*f.vx.Span()
			b := f.vx.Lo + fb*f.vx.Span()
			dx := math.Abs(b - a)
			var reading string
			if f := p.c.axisAt(0).Format; f != nil {
				reading = f(dx)
			} else {
				reading = strconv.FormatFloat(dx, 'g', 4, 64)
			}
			for _, st := range p.states {
				if st.cfg.Hidden || st.d == nil {
					continue
				}
				va, oka := st.d.valueAt(a)
				vb, okb := st.d.valueAt(b)
				if oka && okb {
					reading += "  " + formatValue(float64(vb-va), p.c.axisAt(st.axis).Format)
				}
			}
			return reading
		},
	})
	return out
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
