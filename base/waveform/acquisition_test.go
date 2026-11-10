package waveform

import "testing"

func TestTriggerFindsTheNewestEdge(t *testing.T) {
	d := &data{dx: 1}
	d.append([]float32{0, 1, 2, 4, 4, 3, 2, 0, 3})
	states := []*seriesState{{d: d}}
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	if x, ok := c.triggerAt(states, 0); !ok || x != 8 {
		t.Fatalf("rising edge at %v, %v; want 8", x, ok)
	}
	c.Trigger.Edge = TriggerFalling
	if x, ok := c.triggerAt(states, 0); !ok || x != 6 {
		t.Fatalf("falling edge at %v, %v; want 6", x, ok)
	}
	c.Trigger.Edge = TriggerEither
	if x, ok := c.triggerAt(states, 0); !ok || x != 8 {
		t.Fatalf("either edge at %v, %v; want the newest 8", x, ok)
	}
}

func TestTriggerReportsNoEdge(t *testing.T) {
	d := &data{dx: 1}
	d.append([]float32{0, 1, 2, 3})
	c := Chart{Trigger: &Trigger{Source: 0, Level: 10, Edge: TriggerRising}}
	if _, ok := c.triggerAt([]*seriesState{{d: d}}, 0); ok {
		t.Fatal("a level the signal never reaches found an edge")
	}
}

func TestTriggerSkipsNaNSamples(t *testing.T) {
	// A NaN breaks the line, so a crossing that only spans NaNs is no edge.
	d := &data{dx: 1}
	d.append([]float32{0, 1, nan, nan, 4})
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	if _, ok := c.triggerAt([]*seriesState{{d: d}}, 100); ok {
		t.Fatal("a crossing across a NaN fired")
	}
	// An edge in the finite run before the break still fires.
	d2 := &data{dx: 1}
	d2.append([]float32{0, 1, 3, nan, 4})
	if x, ok := c.triggerAt([]*seriesState{{d: d2}}, 100); !ok || x != 2 {
		t.Fatalf("finite edge before a NaN read %v, %v; want 2", x, ok)
	}
}

func TestTriggerBeforeWaitsForFollowingSamples(t *testing.T) {
	d := &data{dx: 0.001}
	d.append(sineData(1000).y)
	c := Chart{Trigger: &Trigger{Source: 0, Level: 0, Edge: TriggerRising, Position: 0.5}}
	states := []*seriesState{{d: d}}
	e := d.extent()
	limit := e.Hi - 0.2 // leave 200 ms of samples after the edge
	tx, ok := c.triggerAtBefore(states, 0.5, limit)
	if !ok || tx > limit {
		t.Fatalf("edge at %v, %v; want one no later than %v", tx, ok, limit)
	}
	if newest, ok := c.triggerAt(states, 0.5); !ok || newest <= tx {
		t.Fatalf("newest edge %v is not later than the waiting one %v", newest, tx)
	}
}

// widePulses is a long flat line with narrow spikes above the level. A
// striding sweep compares only the samples at each stride's ends, both
// flat, and misses them.
func widePulses(n int, at ...int) *data {
	y := make([]float32, n)
	for _, i := range at {
		y[i] = 5
	}
	d := &data{dx: 1}
	d.append(y)
	return d
}

func TestTriggerFindsANarrowEdgeInAWideView(t *testing.T) {
	const n, at = 200000, 120000 // more than maxTriggerSweep samples
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	x, ok := c.triggerAt([]*seriesState{{d: widePulses(n, at)}}, 100000)
	if !ok || x != at {
		t.Fatalf("a narrow edge in a wide view read %v, %v; want %v", x, ok, at)
	}
}

func TestTriggerEdgeSlopesOverANarrowPulse(t *testing.T) {
	const n, at = 200000, 120000
	d := widePulses(n, at)
	states := []*seriesState{{d: d}}
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5}}
	c.Trigger.Edge = TriggerFalling
	if x, ok := c.triggerAt(states, 100000); !ok || x != at+1 {
		t.Fatalf("the falling edge read %v, %v; want %v", x, ok, at+1)
	}
	c.Trigger.Edge = TriggerEither
	if x, ok := c.triggerAt(states, 100000); !ok || x != at+1 {
		t.Fatalf("either edge read %v, %v; want the newest %v", x, ok, at+1)
	}
}

func TestTriggerFindsTheNewestNarrowEdge(t *testing.T) {
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	states := []*seriesState{{d: widePulses(200000, 100000, 150000)}}
	if x, ok := c.triggerAt(states, 100000); !ok || x != 150000 {
		t.Fatalf("edge read %v, %v; want the newest 150000", x, ok)
	}
}

func TestTriggerFindsAnEdgeAtABucketBoundary(t *testing.T) {
	// Bucket 1 starts at sample 64. Everything from there on stays above
	// the level, so the bucket's own extremes say "no crossing"; only the
	// sample before it makes the rising edge at i=64.
	const n = 100000
	y := make([]float32, n)
	for i := 64; i < n; i++ {
		y[i] = 5
	}
	d := &data{dx: 1}
	d.append(y)
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	if x, ok := c.triggerAt([]*seriesState{{d: d}}, n); !ok || x != 64 {
		t.Fatalf("an edge on a bucket border read %v, %v; want 64", x, ok)
	}
}

func TestTriggerSkipsABucketOfNaNs(t *testing.T) {
	y := make([]float32, 200000)
	for i := range y {
		y[i] = nan
	}
	y[120000] = 5
	d := &data{dx: 1}
	d.append(y)
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	if _, ok := c.triggerAt([]*seriesState{{d: d}}, 100000); ok {
		t.Fatal("a bucket of NaNs fired an edge")
	}
}

func TestTriggerFindsANarrowEdgeWithIrregularX(t *testing.T) {
	// An irregular x has no pyramid, so the search falls back to walking
	// the samples. It must still find an edge a stride would have skipped.
	const n, at = 1000, 600
	y := make([]float32, n)
	y[at] = 5
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i)
	}
	d := &data{y: y, xs: xs, dx: 1}
	c := Chart{Trigger: &Trigger{Source: 0, Level: 2.5, Edge: TriggerRising}}
	if x, ok := c.triggerAt([]*seriesState{{d: d}}, 0); !ok || x != at {
		t.Fatalf("irregular x read %v, %v; want %v", x, ok, at)
	}
}
