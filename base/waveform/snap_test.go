package waveform

import "testing"

func TestSnapFindsPeaksTroughsAndEnds(t *testing.T) {
	d := &data{dx: 1}
	d.append([]float32{0, 1, 5, 1, 0, 1, 5, 1, 0})
	states := []*seriesState{{d: d}}
	vx := Range{0, 8}
	c := Chart{Snap: true}
	const px = 800 // six pixels is 0.06 x units
	if got := c.snapX(2.03, states, vx, px); got != 2 {
		t.Errorf("near a peak: %v, want 2", got)
	}
	if got := c.snapX(0.03, states, vx, px); got != 0 {
		t.Errorf("near the first sample: %v, want 0", got)
	}
	if got := c.snapX(7.97, states, vx, px); got != 8 {
		t.Errorf("near the last sample: %v, want 8", got)
	}
	if got := c.snapX(2.5, states, vx, px); got != 2.5 {
		t.Errorf("away from any point: %v, want 2.5", got)
	}
	if got := (Chart{}).snapX(2.03, states, vx, px); got != 2.03 {
		t.Errorf("snap off moved the point: %v", got)
	}
}

func TestSnapIgnoresAFlatRun(t *testing.T) {
	d := &data{dx: 1}
	d.append([]float32{1, 1, 1, 1})
	c := Chart{Snap: true}
	if got := c.snapX(1.5, []*seriesState{{d: d}}, Range{0, 3}, 800); got != 1.5 {
		t.Fatalf("a flat run snapped to %v", got)
	}
}
