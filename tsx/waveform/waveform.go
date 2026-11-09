// Package waveform gives TSX nsgo/base's and nsgo/ui's Waveform, which
// links base/waveform: an app whose TSX uses it imports this package for
// its effect. A TSX app keeps its columns of samples in a signal and
// passes them as data; a Go app uses a base/waveform.Source instead.
package waveform

import (
"strconv"
	"strings"

	base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

func value[T any](f func() T) T {
	var zero T
	if f == nil {
		return zero
	}
	return f()
}

// Chart is a TSX Waveform's props, base/waveform.Chart's shape without the
// Go-only Source and callbacks. Data is one column of samples per series.
type Chart struct {
	Data    func() [][]float32
	Names   func() []string
	Height  func() float64
	Time    func() bool
	// XFormat is a template for the x axis' labels: "{v}" becomes the
	// tick's value, as "T{v} s" does. A Go function of the value would
	// cross the JS boundary for every tick of every frame, so a TSX app
	// names its words here instead.
	XFormat func() string
	Follow  func() float64
	Cursor  func() bool
	Legend  func() bool
	Measure func() bool
	Label   func() string
	Note    func() string
	Class   string
	Style   core.Style
	TestID  string

	OnCursorChange func(float64)
}

// BaseChart builds base's chart from the props. tsx/ui's styled form reuses
// it, so the two registrations share one shape.
func (c Chart) BaseChart() base.Chart {
	ch := base.Chart{
		Height:  core.Points(value(c.Height)),
		Time:    value(c.Time),
		Follow:  value(c.Follow),
		Cursor:  value(c.Cursor),
		Legend:  value(c.Legend),
		Measure: value(c.Measure),
		Label:   value(c.Label),
		Note:    value(c.Note),
		Class:   c.Class,
		Style:   c.Style,
		TestID:  c.TestID,
	}
	if c.XFormat != nil {
		ch.XFormat = func(v float64) string {
			return strings.ReplaceAll(value(c.XFormat), "{v}",
				strconv.FormatFloat(v, 'g', 4, 64))
		}
	}
	columns := value(c.Data)
	names := value(c.Names)
	for i, y := range columns {
		s := base.Series{Y: y, X0: 0, Dx: 1}
		if i < len(names) {
			s.Name = names[i]
		}
		ch.Series = append(ch.Series, s)
	}
	return ch
}

// Render implements core.Node.
func (c Chart) Render(*core.Scope) core.Node { return c.BaseChart() }

func init() {
	// nsgo/ui's styled form is registered by tsx/ui, the TSX package that
	// may import ui; a TSX app imports this package for the base form.
	tsx.Register("nsgo/base", map[string]core.Node{"Waveform": Chart{}})
}
