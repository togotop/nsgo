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

// Trigger is a TSX Waveform's trigger, base.Trigger with its edge and mode
// named as strings: "rising", "falling" or "either", and "auto", "normal"
// or "single".
type Trigger struct {
	Source   int
	Level    float64
	Edge     string
	Mode     string
	Position float64
}

// Measurement is one automatic reading, its kind named as a string: "vpp",
// "max", "min", "mean", "rms", "frequency", "period", "duty", "rise",
// "fall", "span" or "rate".
type Measurement struct {
	Source int
	Kind   string
	Label  string
	// Over is "view" for the visible window or "markers" for the stretch
	// between the A and B markers.
	Over string
}

// Channel is one series' vertical controls, in the order of Data.
type Channel struct {
	Offset float64
	Scale  float64
	Invert bool
	AC     bool
}

// Chart is a TSX Waveform's props, base/waveform.Chart's shape without the
// Go-only Source, Run and callbacks. Data is one column of samples per
// series.
type Chart struct {
	Data     func() [][]float32
	Names    func() []string
	Channels func() []Channel
	Height   func() float64
	Time     func() bool
	// XFormat is a template for the x axis' labels: "{v}" becomes the
	// tick's value, as "T{v} s" does. A Go function of the value would
	// cross the JS boundary for every tick of every frame, so a TSX app
	// names its words here instead.
	XFormat func() string
	Follow  func() float64
	// TimeBase, DivisionsX and DivisionsY set the scope's window and grid.
	TimeBase     func() float64
	DivisionsX   func() int
	DivisionsY   func() int
	XLog         func() bool
	Cursor       func() bool
	Legend       func() bool
	Measure      func() bool
	MarkLabels   func() []string
	Snap         func() bool
	Trigger      func() *Trigger
	Measurements func() []Measurement
	Label        func() string
	Note         func() string
	Class        string
	Style        core.Style
	TestID       string

	OnCursorChange func(float64)
}

func edgeOf(s string) base.TriggerEdge {
	switch s {
	case "falling":
		return base.TriggerFalling
	case "either":
		return base.TriggerEither
	}
	return base.TriggerRising
}

func modeOf(s string) base.TriggerMode {
	switch s {
	case "normal":
		return base.TriggerNormal
	case "single":
		return base.TriggerSingle
	}
	return base.TriggerAuto
}

func kindOf(s string) base.MeasureKind {
	switch s {
	case "max":
		return base.MeasureMax
	case "min":
		return base.MeasureMin
	case "mean":
		return base.MeasureMean
	case "rms":
		return base.MeasureRMS
	case "frequency":
		return base.MeasureFrequency
	case "period":
		return base.MeasurePeriod
	case "duty":
		return base.MeasureDuty
	case "rise":
		return base.MeasureRise
	case "fall":
		return base.MeasureFall
	case "span":
		return base.MeasureSpan
	case "rate":
		return base.MeasureRate
	}
	return base.MeasureVpp
}

// BaseChart builds base's chart from the props. tsx/ui's styled form reuses
// it, so the two registrations share one shape.
func (c Chart) BaseChart() base.Chart {
	ch := base.Chart{
		Height:     core.Points(value(c.Height)),
		Time:       value(c.Time),
		Follow:     value(c.Follow),
		TimeBase:   value(c.TimeBase),
		DivisionsX: value(c.DivisionsX),
		DivisionsY: value(c.DivisionsY),
		XLog:       value(c.XLog),
		Cursor:     value(c.Cursor),
		Legend:     value(c.Legend),
		Measure:    value(c.Measure),
		Snap:       value(c.Snap),
		Label:      value(c.Label),
		Note:       value(c.Note),
		Class:      c.Class,
		Style:      c.Style,
		TestID:     c.TestID,
	}
	if c.XFormat != nil {
		ch.XFormat = func(v float64) string {
			return strings.ReplaceAll(value(c.XFormat), "{v}",
				strconv.FormatFloat(v, 'g', 4, 64))
		}
	}
	if t := value(c.Trigger); t != nil {
		ch.Trigger = &base.Trigger{Source: t.Source, Level: t.Level,
			Edge: edgeOf(t.Edge), Mode: modeOf(t.Mode), Position: t.Position}
	}
	for _, m := range value(c.Measurements) {
		over := base.MeasureView
		if m.Over == "markers" {
			over = base.MeasureMarkers
		}
		ch.Measurements = append(ch.Measurements, base.Measurement{
			Source: m.Source, Kind: kindOf(m.Kind), Label: m.Label, Over: over})
	}
	labels := value(c.MarkLabels)
	for i := 0; i < len(ch.MarkLabels) && i < len(labels); i++ {
		ch.MarkLabels[i] = labels[i]
	}
	columns := value(c.Data)
	names := value(c.Names)
	channels := value(c.Channels)
	for i, y := range columns {
		s := base.Series{Y: y, X0: 0, Dx: 1}
		if i < len(names) {
			s.Name = names[i]
		}
		if i < len(channels) {
			s.Offset = channels[i].Offset
			s.Scale = channels[i].Scale
			s.Invert = channels[i].Invert
			s.AC = channels[i].AC
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
