package waveform

import "math"

// snapX moves x onto a nearby peak, trough or end of a visible series, so a
// cursor or a marker settles on a meaningful point rather than a sample in
// between. Nothing within a few pixels of x leaves it where it was.
func (c Chart) snapX(x float64, states []*seriesState, vx Range, px int) float64 {
	if !c.Snap || px <= 0 || !vx.Valid() {
		return x
	}
	radius := vx.Span() / float64(px) * 6
	if c.XLog && vx.Lo > 0 {
		// On a log axis a pixel is not a fixed span of data x: measure six
		// pixels around x in the view's own scale.
		f := c.xFrac(x, vx)
		lo := c.xAtFrac(min(max(f-6/float64(px), 0), 1), vx)
		hi := c.xAtFrac(min(max(f+6/float64(px), 0), 1), vx)
		radius = max(math.Abs(x-lo), math.Abs(hi-x))
	}
	best, bestD := x, radius
	consider := func(v float64) {
		if d := math.Abs(v - x); d < bestD {
			best, bestD = v, d
		}
	}
	for _, st := range states {
		if st.cfg.Hidden || st.d == nil || len(st.d.y) < 3 {
			continue
		}
		e := st.d.extent()
		for _, end := range [...]float64{e.Lo, e.Hi} {
			if end >= vx.Lo && end <= vx.Hi {
				consider(end)
			}
		}
		from := max(1, st.d.indexAt(x-radius))
		to := min(len(st.d.y)-2, st.d.indexAt(x+radius))
		for i := from; i <= to; i++ {
			a, b, n := st.d.y[i-1], st.d.y[i], st.d.y[i+1]
			if a != a || b != b || n != n {
				continue
			}
			// A strict side keeps a flat run from snapping everywhere.
			if b > a && b >= n || b < a && b <= n {
				consider(st.d.xAt(i))
			}
		}
	}
	return best
}
