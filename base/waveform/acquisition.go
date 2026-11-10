package waveform

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// maxTriggerSweep bounds how many pyramid buckets an edge search sweeps.
// A level whose bucket count fits under it keeps a wide view's search
// cheap without striding past narrow edges.
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

// triggerLevel picks the finest pyramid level whose bucket count stays
// under maxTriggerSweep, or -1 when the column has no pyramid (an
// irregular x, or too few samples). Those buckets are coarse enough to
// bound a wide sweep but fine enough that a hit is cheap to verify.
func triggerLevel(n, levels int) int {
	if levels == 0 {
		return -1
	}
	for l, sh := range bucketShifts {
		if l >= levels {
			break
		}
		if n>>sh <= maxTriggerSweep {
			return l
		}
	}
	return levels - 1
}

// triggerInSamples walks the samples in (from, to) from the newest back,
// and returns the x of the first that crosses level on the edge's slope.
// A NaN on either side breaks the line there, so it is no edge.
func triggerInSamples(d *data, from, to int, level float64, edge TriggerEdge) (float64, bool) {
	for i := to - 1; i > from; i-- {
		a, b := d.y[i-1], d.y[i]
		if a != a || b != b {
			continue
		}
		up := float64(a) < level && float64(b) >= level
		down := float64(a) >= level && float64(b) < level
		var hit bool
		switch edge {
		case TriggerRising:
			hit = up
		case TriggerFalling:
			hit = down
		default:
			hit = up || down
		}
		if hit {
			return d.xAt(i), true
		}
	}
	return 0, false
}

// triggerAtBefore is triggerAt over edges no later than limit, so a chart
// waits until enough samples follow an edge to fill the view. A wide view
// is swept through the pyramid's buckets: a bucket's extremes say whether
// it can hold an edge at all, and only those are searched sample by
// sample, so a narrow edge deep in a long history is never skipped.
func (c Chart) triggerAtBefore(states []*seriesState, width, limit float64) (float64, bool) {
	t := c.Trigger
	if t == nil || t.Source < 0 || t.Source >= len(states) {
		return 0, false
	}
	st := states[t.Source]
	if st.d == nil || len(st.d.y) < 2 {
		return 0, false
	}
	d := st.d
	e := d.extent()
	if !e.Valid() {
		return 0, false
	}
	span := width
	if span <= 0 {
		span = e.Span() / 4
	}
	from := d.indexAt(e.Hi - 2*span)
	to := len(d.y)
	if limit < e.Hi {
		to = min(to, d.indexAt(limit)+1)
	}
	level := t.Level

	lvl := triggerLevel(len(d.y), len(d.py.levels))
	if lvl < 0 {
		return triggerInSamples(d, from, to, level, t.Edge)
	}
	shift := bucketShifts[lvl]
	buckets := d.py.levels[lvl]
	for b := len(buckets) - 1; b >= 0; b-- {
		lo := b << shift
		if lo >= to {
			continue
		}
		hi := min(lo+(1<<shift), len(d.y))
		if hi <= from {
			break
		}
		mm := buckets[b]
		if mm.lo != mm.lo { // every sample in the bucket is NaN
			continue
		}
		// An edge may straddle the bucket's left border: y[lo-1] is in
		// the bucket before it. Include that sample in the extremes, or
		// a crossing that lands exactly on a border would be skipped.
		loVal, hiVal := float64(mm.lo), float64(mm.hi)
		if lo > 0 {
			if p := d.y[lo-1]; p == p {
				f := float64(p)
				loVal, hiVal = min(loVal, f), max(hiVal, f)
			}
		}
		if hiVal < level || loVal >= level {
			continue
		}
		start := max(lo-1, from)
		if x, ok := triggerInSamples(d, start, min(hi, to), level, t.Edge); ok {
			return x, true
		}
	}
	return 0, false
}
