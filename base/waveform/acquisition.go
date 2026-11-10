package waveform

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// maxTriggerSweep is the most sample positions an edge search looks at.
const maxTriggerSweep = 1 << 16

// TriggerEdge is the slope a trigger fires on.
type TriggerEdge uint8

const (
	// TriggerRising fires when the signal crosses Level going up. It is the
	// zero value.
	TriggerRising TriggerEdge = iota
	// TriggerFalling fires when the signal crosses Level going down.
	TriggerFalling
	// TriggerEither fires on either crossing.
	TriggerEither
)

// TriggerMode says what a chart shows when no edge is found.
type TriggerMode uint8

const (
	// TriggerAuto keeps showing new samples, as an untriggered chart does.
	// It is the zero value.
	TriggerAuto TriggerMode = iota
	// TriggerNormal holds the last triggered view until a new edge arrives.
	TriggerNormal
	// TriggerSingle holds the first view an edge anchors, taking one
	// capture. A double click re-arms it.
	TriggerSingle
)

// Trigger pins one series' edge at Position in the view, so a repeating
// waveform stands still as new samples arrive. Without a trigger a chart
// follows its newest samples or its view.
type Trigger struct {
	// Source names the index into Chart.Series whose edge is watched.
	Source int
	// Level is the value the signal has to cross, in the series' units.
	Level float64
	Edge  TriggerEdge
	Mode  TriggerMode
	// Position is where the edge sits across the view, in (0, 1]: 0.5 is
	// the middle, 1 the right. Zero (the default) also means the middle,
	// so an app that wants the edge at the left uses a small value.
	Position float64
}

// Run controls a chart's acquisition: Stop freezes a copy of the samples
// it shows, which the view still zooms and pans, Start resumes, and Single
// arms one capture that stops after the next edge. A chart without a Run
// keeps running.
//
// A Run is armed when its chart mounts; call it from an app's controls, as
// a scope's Run/Stop and Single buttons do.
type Run struct {
	running *core.Signal[bool]
	single  *core.Signal[bool]
}

// attach gives the Run its signals the first time its chart renders.
func (r *Run) attach(cx *core.Scope) {
	if r.running == nil {
		r.running = core.NewSignal(cx, true)
		r.single = core.NewSignal(cx, false)
	}
}

// Stop freezes the chart at the frame it last drew.
func (r *Run) Stop() {
	if r != nil && r.running != nil {
		r.running.Set(false)
	}
}

// Start resumes a stopped chart.
func (r *Run) Start() {
	if r != nil && r.running != nil {
		r.running.Set(true)
	}
}

// Single resumes and arms one capture: the chart runs until the next
// trigger edge, then stops.
func (r *Run) Single() {
	if r != nil && r.single != nil {
		r.running.Set(true)
		r.single.Set(true)
	}
}

// Running reports whether the chart is acquiring.
func (r *Run) Running() bool {
	return r == nil || r.running == nil || r.running.GetUntracked()
}

// triggerAt finds the newest edge of the trigger's series within width x
// units of its newest sample, and returns the x where it crossed.
func (c Chart) triggerAt(states []*seriesState, width float64) (float64, bool) {
	return c.triggerAtBefore(states, width, math.Inf(1))
}

// triggerAtBefore is triggerAt over edges no later than limit, so a chart
// waits until enough samples follow an edge to fill the view.
func (c Chart) triggerAtBefore(states []*seriesState, width, limit float64) (float64, bool) {
	t := c.Trigger
	if t == nil || t.Source < 0 || t.Source >= len(states) {
		return 0, false
	}
	st := states[t.Source]
	if st.d == nil || len(st.d.y) < 2 {
		return 0, false
	}
	e := st.d.extent()
	if !e.Valid() {
		return 0, false
	}
	span := width
	if span <= 0 {
		span = e.Span() / 4
	}
	from := st.d.indexAt(e.Hi - 2*span)
	to := len(st.d.y)
	if limit < e.Hi {
		to = min(to, st.d.indexAt(limit)+1)
	}
	level := t.Level
	// A wide view can hold a long history. Cap the sweep at a fixed number
	// of looks, sampling in strides when it is longer, so finding an edge
	// never walks every retained sample.
	step := 1
	if sweep := to - from; sweep > maxTriggerSweep {
		step = (sweep + maxTriggerSweep - 1) / maxTriggerSweep
	}
	for i := to - 1; i > from; i -= step {
		j := max(i-step, from)
		a, b := st.d.y[j], st.d.y[i]
		if a != a || b != b {
			continue
		}
		up := float64(a) < level && float64(b) >= level
		down := float64(a) >= level && float64(b) < level
		var hit bool
		switch t.Edge {
		case TriggerRising:
			hit = up
		case TriggerFalling:
			hit = down
		default:
			hit = up || down
		}
		if hit {
			return st.d.xAt(i), true
		}
	}
	return 0, false
}
