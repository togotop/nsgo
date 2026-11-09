package waveform

import (
	"math"
	"testing"
)

// samples returns n samples of two tones, with a spike every 97 samples so
// a pixel column's lowest and highest differ and decimation must keep both.
func samples(n int) []float32 {
	y := make([]float32, n)
	for i := range y {
		y[i] = float32(200 + 110*math.Sin(float64(i)/37) + 45*math.Sin(float64(i)/3.1))
		if i%97 == 0 {
			y[i] += 300
		}
	}
	return y
}

func columnData(n int) *data {
	d := &data{dx: 0.001}
	d.append(samples(n))
	return d
}

func TestColumnsKeepsEachColumnsExtremes(t *testing.T) {
	const px = 128
	d := columnData(10000)
	vx := d.extent()
	// The exact per-column extremes hold for the raw path; a bucketed scan
	// places a bucket at its first sample's column, which is the LOD's
	// approximation, checked separately.
	raw := scan{kind: scanRaw, y: d.y, x0: d.x0, dx: d.dx, n: len(d.y)}
	pts := columns(raw, vx, px, nil)
	type pair struct {
		lo, hi float32
		ok     bool
	}
	got := make([]pair, px)
	for i := 0; i+1 < len(pts); i += 2 {
		c, v := int(pts[i]), pts[i+1]
		if c < 0 || c >= px {
			continue
		}
		if !got[c].ok {
			got[c] = pair{v, v, true}
		} else {
			got[c].lo = min(got[c].lo, v)
			got[c].hi = max(got[c].hi, v)
		}
	}
	// Use the same column mapping as columns, so a boundary sample lands
	// in the column the decimator put it in.
	scale := float64(px) / vx.Span()
	col := func(i int) int { return int(math.Floor((d.xAt(i) - vx.Lo) * scale)) }
	for c := 0; c < px; c++ {
		lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
		found := false
		for i := range d.y {
			if col(i) != c {
				continue
			}
			found = true
			lo, hi = min(lo, d.y[i]), max(hi, d.y[i])
		}
		if !found {
			continue
		}
		if !got[c].ok {
			t.Fatalf("column %d has no point", c)
		}
		if got[c].lo != lo || got[c].hi != hi {
			t.Fatalf("column %d: got %v..%v, want %v..%v", c, got[c].lo, got[c].hi, lo, hi)
		}
	}
}

func TestPyramidLevelsMatchRawSamples(t *testing.T) {
	d := columnData(100000)
	for l, sh := range bucketShifts {
		if l >= len(d.py.levels) {
			break
		}
		buckets := d.py.levels[l]
		for b := range buckets {
			from := b << sh
			to := min((b+1)<<sh, len(d.y))
			want := extreme(d.y, from, to)
			if buckets[b] != want {
				t.Fatalf("level %d bucket %d: got %v, want %v", l, b, buckets[b], want)
			}
		}
	}
}

func TestPyramidExtendsOnlyNewBuckets(t *testing.T) {
	d := &data{dx: 0.001}
	d.append(samples(10000))
	before := make([]minmax, len(d.py.levels[0]))
	copy(before, d.py.levels[0])
	d.append(samples(10000))
	// The buckets before the append are unchanged; only the last partial
	// one and the new ones are rebuilt.
	for b := range before {
		if b < len(before)-1 && d.py.levels[0][b] != before[b] {
			t.Fatalf("bucket %d changed without new samples", b)
		}
	}
	if len(d.y) != 20000 || len(d.py.levels[0]) != (20000+63)/64 {
		t.Fatalf("append did not grow the column: %d samples, %d buckets", len(d.y), len(d.py.levels[0]))
	}
}

func TestDecimationIsOWidth(t *testing.T) {
	const px = 1024
	for _, n := range []int{100000, 1000000} {
		d := columnData(n)
		vx := d.extent()
		sc := d.scanFor(vx, px)
		pts := columns(sc, vx, px, nil)
		if points := len(pts) / 2; points > 2*px+32 {
			t.Errorf("%d samples: %d points, want at most about %d", n, points, 2*px)
		}
		if sc.kind != scanBuckets || sc.n > 4*px {
			t.Errorf("%d samples: scan is %v with %d items, want buckets at most %d", n, sc.kind, sc.n, 4*px)
		}
	}
}

func TestReuseOfGeom(t *testing.T) {
	const px = 256
	d := columnData(10000)
	vx := d.extent()
	key := geomKey{lo: vx.Lo, hi: vx.Hi, px: px, version: d.version, n: len(d.y), x0: d.x0, dx: d.dx}
	var g geom
	first := g.get(key, d.scanFor(vx, px), vx, px)
	firstCopy := append([]float32(nil), first...)
	again := g.get(key, d.scanFor(vx, px), vx, px)
	if len(again) != len(firstCopy) {
		t.Fatal("a reused geom changed length")
	}
	key.version++
	_ = g.get(key, d.scanFor(vx, px), vx, px)
	if len(g.points) == 0 {
		t.Fatal("a new version did not decimate again")
	}
}

func TestNaNsBreakTheLine(t *testing.T) {
	d := &data{dx: 1}
	y := samples(1000)
	for i := 300; i < 320; i++ {
		y[i] = nan
	}
	d.append(y)
	vx := d.extent()
	pts := columns(d.scanFor(vx, 100), vx, 100, nil)
	// The finite runs, split by the NaN pair a break writes.
	runs, inRun := 0, false
	for i := 0; i+1 < len(pts); i += 2 {
		finite := pts[i] == pts[i] && pts[i+1] == pts[i+1]
		switch {
		case finite && !inRun:
			runs, inRun = runs+1, true
		case !finite:
			inRun = false
		}
	}
	if runs != 2 {
		t.Fatalf("a NaN stretch split the line into %d runs, want 2", runs)
	}
}

func TestIrregularXScansRawSamples(t *testing.T) {
	d := &data{dx: 1}
	d.y = samples(1000)
	d.xs = make([]float64, len(d.y))
	for i := range d.xs {
		d.xs[i] = float64(i) * float64(i) * 0.001
	}
	vx := d.extent()
	sc := d.scanFor(vx, 100)
	if sc.kind != scanRaw {
		t.Fatal("an irregular x used buckets, which assume a uniform grid")
	}
	pts := columns(sc, vx, 100, nil)
	if len(pts) == 0 {
		t.Fatal("an irregular x drew nothing")
	}
}

func TestViewport(t *testing.T) {
	v := Viewport{X: Range{0, 10}}
	if got := v.ZoomX(0.5, 5).X; got.Lo != 2.5 || got.Hi != 7.5 {
		t.Errorf("ZoomX: %v", got)
	}
	if got := v.PanX(2).X; got.Lo != 2 || got.Hi != 12 {
		t.Errorf("PanX: %v", got)
	}
	fit := Viewport{}.Fit(Range{5, 15})
	if fit.X != (Range{5, 15}) {
		t.Errorf("Fit: %v", fit.X)
	}
	clamped := Viewport{X: Range{-100, -90}}.Clamp(Range{0, 10})
	if clamped.X.Lo != 0 {
		t.Errorf("Clamp: %v", clamped.X)
	}
}

func TestTicks(t *testing.T) {
	a := Axis{}
	linear := a.ticks(0, 1, 5)
	if len(linear) < 2 || linear[0].At < 0 || linear[len(linear)-1].At > 1 {
		t.Errorf("linear ticks: %v", linear)
	}
	logA := Axis{Log: true}
	if got := logA.ticks(1, 10000, 6); len(got) < 2 {
		t.Errorf("log ticks: %v", got)
	}
	timed := timeTicks(0, 60, 6, nil)
	if len(timed) < 2 || timed[0].Label == "" {
		t.Errorf("time ticks: %v", timed)
	}
}

func TestTrimDropsOldSamples(t *testing.T) {
	d := columnData(10000)
	d.trim(3) // x of sample 3000
	if len(d.y) != 7000 || d.x0 != 3 {
		t.Fatalf("trim left %d samples at x0 %v", len(d.y), d.x0)
	}
	if got := d.extent(); got.Lo != 3 {
		t.Errorf("extent after trim: %v", got)
	}
}

func TestSourceRetainBoundsHistory(t *testing.T) {
	src := NewSource(0, 0.001)
	src.Retain(1) // one x unit, 1000 samples
	for b := range 10 {
		y := make([]float32, 1000)
		for i := range y {
			y[i] = float32(b*1000 + i)
		}
		src.Push(y)
		src.adopt() // as the engine does, without a host
	}
	if e := src.d.extent(); e.Span() > 2 {
		t.Fatalf("history grew past twice the retained span: %v", e)
	}
	if n := len(src.d.y); n == 0 || src.d.y[n-1] != 9999 {
		t.Fatalf("the newest sample was lost: %d samples, last %v", n, src.d.y[n-1])
	}
	if len(src.d.py.levels[0]) > 64 {
		t.Fatalf("the pyramid grew to %d buckets", len(src.d.py.levels[0]))
	}
}

func TestSourceWithoutRetainKeepsEverything(t *testing.T) {
	src := NewSource(0, 1)
	src.Push(make([]float32, 100))
	src.adopt()
	if len(src.d.y) != 100 {
		t.Fatalf("an unretained Source dropped samples: %d", len(src.d.y))
	}
}

func TestSeriesHeadRotatesTheRing(t *testing.T) {
	c := Chart{Series: []Series{{Y: []float32{1, 2, 3, 4}, X0: 0, Dx: 1, Head: 2}}}
	got := c.states()[0].d.y
	if len(got) != 4 || got[0] != 3 || got[1] != 4 || got[2] != 1 || got[3] != 2 {
		t.Fatalf("the ring rotated to %v", got)
	}
}

func TestSourceResetStartsANewColumn(t *testing.T) {
	src := NewSource(1, 0.5)
	src.Push(make([]float32, 10))
	src.adopt()
	src.Reset(5, 0.25)
	if len(src.d.y) != 0 || src.d.x0 != 5 || src.d.dx != 0.25 {
		t.Fatalf("reset left %d samples at x0 %v dx %v", len(src.d.y), src.d.x0, src.d.dx)
	}
}

func TestSourceRetainKeepsTheWidest(t *testing.T) {
	src := NewSource(0, 1)
	src.Retain(2)
	src.Retain(1)
	if src.retain != 2 {
		t.Fatalf("retain became %v, want the widest 2", src.retain)
	}
}

func TestValueAt(t *testing.T) {
	d := columnData(1000)
	v, ok := d.valueAt(0.5)
	if !ok || v != d.y[500] {
		t.Fatalf("valueAt: %v, %v", v, ok)
	}
	if _, ok := d.valueAt(-1); ok {
		t.Fatal("a value before the data read as present")
	}
}
