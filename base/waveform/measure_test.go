package waveform

import (
	"math"
	"testing"
)

func sineData(n int) *data {
	d := &data{dx: 0.001}
	y := make([]float32, n)
	for i := range y {
		y[i] = float32(100 * math.Sin(2*math.Pi*float64(i)/100))
	}
	d.append(y)
	return d
}

// pointsOf is the full, undrawn column of a series, for tests that check
// the measurement itself rather than the reduced points a chart feeds it.
func pointsOf(d *data) ([]float64, []float64) {
	xs := make([]float64, len(d.y))
	ys := make([]float64, len(d.y))
	for i := range d.y {
		xs[i], ys[i] = d.xAt(i), float64(d.y[i])
	}
	return xs, ys
}

func TestMeasureReadsASine(t *testing.T) {
	d := sineData(1000)
	xs, ys := pointsOf(d)
	vx := d.extent()
	cases := []struct {
		kind      MeasureKind
		want, tol float64
	}{
		{MeasureVpp, 200, 1},
		{MeasureMax, 100, 1},
		{MeasureMin, -100, 1},
		{MeasureMean, 0, 1},
		{MeasureRMS, 100 / math.Sqrt2, 2},
		{MeasurePeriod, 0.1, 0.002},
		{MeasureFrequency, 10, 0.2},
		{MeasureDuty, 0.5, 0.02},
		{MeasureRise, 0.03, 0.02},
		{MeasureFall, 0.03, 0.02},
	}
	for _, tc := range cases {
		got, ok := measureFrom(Measurement{Kind: tc.kind}, xs, ys, vx)
		if !ok {
			t.Fatalf("%v: no reading", tc.kind)
		}
		if math.Abs(got-tc.want) > tc.tol {
			t.Errorf("%v: got %v, want %v ± %v", tc.kind, got, tc.want, tc.tol)
		}
	}
}

func TestMeasureNeedsEnoughOfASeries(t *testing.T) {
	xs, ys := []float64{0, 1}, []float64{1, 2}
	for _, kind := range []MeasureKind{MeasureFrequency, MeasurePeriod, MeasureRise, MeasureFall} {
		if _, ok := measureFrom(Measurement{Kind: kind}, xs, ys, Range{0, 1}); ok {
			t.Errorf("%v read from two samples", kind)
		}
	}
	if _, ok := measureFrom(Measurement{Source: -1, Kind: MeasureVpp}, xs, ys, Range{0, 1}); ok {
		t.Error("a missing series gave a reading")
	}
}

func TestMeasureSpanAndRate(t *testing.T) {
	vx := Range{2, 4}
	if v, ok := measureFrom(Measurement{Kind: MeasureSpan}, nil, nil, vx); !ok || v != 2 {
		t.Fatalf("span: %v, %v; want 2", v, ok)
	}
	if v, ok := measureFrom(Measurement{Kind: MeasureRate}, nil, nil, vx); !ok || v != 0.5 {
		t.Fatalf("rate: %v, %v; want 0.5", v, ok)
	}
}

// A reading fed the drawn points stays exact on the extremes and takes no
// more points than a frame draws, however much history the source keeps.
func TestMeasureFromReducedPointsKeepsExtremes(t *testing.T) {
	d := columnData(1000000)
	vx := d.extent()
	const px = 600
	pts := columns(d.scanFor(vx, px, false), vx, px, false, nil)
	xs := make([]float64, 0, len(pts)/2)
	ys := make([]float64, 0, len(pts)/2)
	for i := 0; i+1 < len(pts); i += 2 {
		col, v := float64(pts[i]), float64(pts[i+1])
		if col < 0 || col > px || v != v {
			continue
		}
		xs = append(xs, vx.Lo+col/px*vx.Span())
		ys = append(ys, v)
	}
	if len(ys) > 2*px+32 {
		t.Fatalf("a reading took %d points, want about twice the width", len(ys))
	}
	want := float64(extreme(d.y, 0, len(d.y)).hi - extreme(d.y, 0, len(d.y)).lo)
	got, ok := measureFrom(Measurement{Kind: MeasureVpp}, xs, ys, vx)
	if !ok || math.Abs(got-want) > 0.01 {
		t.Fatalf("peak-to-peak from reduced points: %v, %v; want %v", got, ok, want)
	}
}
