package waveform

import (
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/core"
)

// batch is a column published from another goroutine. Batches chain so
// several producers may push at once without a lock.
type batch struct {
	y    []float32
	next *batch
}

// Source is where an app's device thread publishes samples. Push is safe
// from any goroutine; a Chart adopts what accumulated on the engine's
// goroutine, so a burst of pushes costs one commit, not one a sample.
//
// A Source belongs to one Chart for its lifetime. Its samples are spaced
// by dx from x0; build a new Source (or call Reset on the engine) when the
// device changes its rate.
type Source struct {
	head   atomic.Pointer[batch]
	posted atomic.Bool
	cx     atomic.Pointer[core.Scope]

	// Engine-only: changed wakes the chart's drawing, d holds the samples.
	changed *core.Signal[uint64]
	d       data
}

// NewSource starts a column of samples spaced by dx from x0.
func NewSource(x0, dx float64) *Source {
	if dx <= 0 {
		dx = 1
	}
	return &Source{d: data{x0: x0, dx: dx}}
}

// Push publishes samples from any goroutine. The slice must not be written
// after the call: build the next one afresh. Pushes queue one adopt for the
// whole burst, and never block on the engine.
func (s *Source) Push(y []float32) {
	if len(y) == 0 {
		return
	}
	b := &batch{y: y}
	for {
		old := s.head.Load()
		b.next = old
		if s.head.CompareAndSwap(old, b) {
			break
		}
	}
	if s.posted.CompareAndSwap(false, true) {
		if cx := s.cx.Load(); cx != nil {
			cx.Post(s.adopt)
		} else {
			// No host yet: attach posts one, and this batch stays queued.
			s.posted.Store(false)
		}
	}
}

// Reset drops every sample and starts a new column. Call it on the engine.
func (s *Source) Reset(x0, dx float64) {
	if dx <= 0 {
		dx = 1
	}
	s.d.reset(x0, dx)
	if s.changed != nil {
		s.changed.Set(s.d.version)
	}
}

// attach gives the Source its host the first time a Chart renders. A push
// made before any view existed is adopted by the post below.
func (s *Source) attach(cx *core.Scope) {
	if s.changed == nil {
		s.changed = core.NewSignal(cx, uint64(0))
	}
	s.cx.Store(cx)
	cx.Post(s.adopt)
}

// adopt runs on the engine: it takes the whole chain of published batches,
// appends them in order, and wakes the drawing once. Clearing posted before
// taking the chain lets a push that lands during the adopt queue another.
func (s *Source) adopt() {
	s.posted.Store(false)
	list := s.head.Swap(nil)
	if list == nil {
		return
	}
	var columns [][]float32
	for b := list; b != nil; b = b.next {
		columns = append(columns, b.y)
	}
	for i, j := 0, len(columns)-1; i < j; i, j = i+1, j-1 {
		columns[i], columns[j] = columns[j], columns[i]
	}
	for _, y := range columns {
		s.d.append(y)
	}
	if s.changed != nil {
		s.changed.Set(s.d.version)
	}
}
