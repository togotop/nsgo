package waveform

import (
	"math"
	"testing"
)

func TestChartXFracOnALogAxis(t *testing.T) {
	c := Chart{XLog: true}
	vx := Range{1, 1000}
	if got := c.xFrac(10, vx); math.Abs(got-1.0/3) > 1e-9 {
		t.Fatalf("x=10 across 1..1000 is at %v, want 1/3", got)
	}
	if got := c.xAtFrac(1.0/3, vx); math.Abs(got-10) > 1e-6 {
		t.Fatalf("1/3 across 1..1000 is at %v, want 10", got)
	}
	if got := (Chart{}).xFrac(10, Range{0, 100}); got != 0.1 {
		t.Fatalf("a linear axis put 10 at %v, want 0.1", got)
	}
}

// On a log axis the columns at the right cover far more samples than those
// at the left. The scan must account for that density instead of choosing a
// level from the linear average, and still keep the drawn points bounded.
func TestLogXScanBalancesColumnDensity(t *testing.T) {
	d := columnData(1000000)
	vx := Range{1, d.extent().Hi}
	const px = 600
	linear := d.scanFor(vx, px, false)
	balanced := d.scanFor(vx, px, true)
	if linear.kind != scanBuckets || balanced.kind != scanBuckets {
		t.Fatalf("scans: linear %v, log %v, want buckets", linear.kind, balanced.kind)
	}
	if balanced.shift < linear.shift {
		t.Fatalf("a log scan chose finer buckets (%d) than the linear average (%d)",
			balanced.shift, linear.shift)
	}
	pts := columns(balanced, vx, px, true, nil)
	if n := len(pts) / 2; n > 2*px+32 {
		t.Fatalf("a log scan drew %d points, want about twice the width", n)
	}
}
