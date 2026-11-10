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
