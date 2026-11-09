package waveform

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// Series is one line of a Chart: the samples it draws and how it draws
// them. Either Source feeds a live column, or Y holds a static one with
// X0/Dx (uniform) or X (irregular, ascending). Head rotates a static Y as
// a ring: sample i sits at Y[(Head+i)%len(Y)].
//
// Y and X are read when the chart mounts: to change what a running chart
// shows, push to a Source instead of rewriting a column.
//
// Axis names the index into Chart.Axes the values are measured on. Color,
// Width and Fill are shortcuts; Class, Style and StyleFunc style the line
// as any other part, and win over them.
type Series struct {
	Name   string
	Source *Source
	Y      []float32
	X      []float64
	X0, Dx float64
	Head   int
	Axis   int

	Color  core.Color
	Width  core.Points
	Fill   core.Color
	Hidden bool

	Class     string
	Style     core.Style
	StyleFunc func() core.Style
}

// Band shades a range of y across the plot, under the series: a tolerance
// around a value, a safe operating area, a target range.
type Band struct {
	Lo, Hi float64
	Axis   int
	Color  core.Color
	Label  string
}

// Note is a label at a point of the data, such as "trigger" or "sensor
// swapped": a dot on the point and its text beside it, panning and zooming
// with the data. A note outside the view is not drawn.
type Note struct {
	X, Y  float64
	Axis  int
	Text  string
	Color core.Color
}

// Group shares one x view and one cursor across the charts that name it,
// as uPlot's sync does: a drag in one zooms them all, and the crosshairs
// line up. Each chart still fits its own y range to its own series.
type Group struct {
	view   *core.Signal[Viewport]
	cursor *core.Signal[float64]
}

// attach makes the group's signals on the first chart that names it.
func (g *Group) attach(cx *core.Scope) {
	if g.view == nil {
		g.view = core.NewSignal(cx, Viewport{})
		g.cursor = core.NewSignal(cx, math.NaN())
	}
}

func (g *Group) viewOf(fallback Viewport) Viewport {
	if g == nil || g.view == nil {
		return fallback
	}
	v := g.view.Get()
	if !v.X.Valid() {
		return fallback
	}
	return v
}

func (g *Group) setView(v Viewport) {
	if g != nil && g.view != nil {
		g.view.Set(v)
	}
}

func (g *Group) cursorOf() float64 {
	if g == nil || g.cursor == nil {
		return math.NaN()
	}
	return g.cursor.Get()
}

func (g *Group) setCursor(x float64) {
	if g != nil && g.cursor != nil {
		g.cursor.Set(x)
	}
}
