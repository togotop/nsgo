package waveform

import (
	"math"
	"sort"
)

// data is one series' samples and the pyramid over them. It belongs to the
// engine: a Source adopts new samples into it on the graph's goroutine, and
// a Chart reads it while drawing. Everything here is pure Go, so the
// decimator builds and tests without a platform.
type data struct {
	y  []float32
	xs []float64 // irregular x, ascending; nil means x0+i*dx
	x0 float64
	dx float64

	version uint64
	py      pyramid
}

// append adds samples at the end and updates only the buckets they reach.
func (d *data) append(y []float32) {
	from := len(d.y)
	d.y = append(d.y, y...)
	if d.xs == nil {
		d.py.extend(d.y, from, d.x0, d.dx)
	}
	d.version++
}

// reset drops every sample and starts a new column at x0, spaced by dx.
func (d *data) reset(x0, dx float64) {
	d.y = d.y[:0]
	d.xs = nil
	d.x0, d.dx = x0, dx
	d.py.levels = d.py.levels[:0]
	d.py.n = 0
	d.version++
}

// trim drops the samples before x. It is O(n), so an app that follows a
// live window calls it for a whole stretch at once, not every frame.
func (d *data) trim(before float64) {
	i := d.indexAt(before)
	if i <= 0 {
		return
	}
	d.y = append(d.y[:0], d.y[i:]...)
	if d.xs != nil {
		d.xs = append(d.xs[:0], d.xs[i:]...)
	}
	d.x0 += float64(i) * d.dx
	d.py.extend(d.y, 0, d.x0, d.dx)
	d.version++
}

func (d *data) xAt(i int) float64 {
	if d.xs != nil {
		return d.xs[i]
	}
	return d.x0 + float64(i)*d.dx
}

// indexAt returns the index of the first sample at or after x, clamped.
func (d *data) indexAt(x float64) int {
	n := len(d.y)
	if n == 0 {
		return 0
	}
	if d.xs != nil {
		i := sort.Search(n, func(i int) bool { return d.xs[i] >= x })
		return min(max(i, 0), n-1)
	}
	if d.dx <= 0 {
		return 0
	}
	i := int(math.Ceil((x - d.x0) / d.dx))
	return min(max(i, 0), n-1)
}

// clone is a copy of the samples and their pyramid, which a stopped chart
// draws while its source keeps filling.
func (d *data) clone() *data {
	c := &data{x0: d.x0, dx: d.dx, version: d.version}
	c.y = append([]float32(nil), d.y...)
	if d.xs != nil {
		c.xs = append([]float64(nil), d.xs...)
	} else {
		c.py.extend(c.y, 0, c.x0, c.dx)
	}
	return c
}

// extent is the x range the samples cover.
func (d *data) extent() Range {
	if len(d.y) == 0 {
		return Range{}
	}
	return Range{d.xAt(0), d.xAt(len(d.y) - 1)}
}

// valueAt is the sample nearest x, for a cursor's reading. A NaN sample
// reports none, so a break in the line reads as no value.
func (d *data) valueAt(x float64) (float32, bool) {
	if len(d.y) == 0 {
		return 0, false
	}
	e := d.extent()
	if x < e.Lo || x > e.Hi {
		return 0, false
	}
	i := d.indexAt(x)
	v := d.y[i]
	return v, v == v
}

// scanFor returns how to walk the column for vx at px pixels wide: the raw
// samples when the view shows few of them, else the buckets of the level
// that fits a pixel column. An irregular x has no bucket grid.
func (d *data) scanFor(vx Range, px int, logX bool) scan {
	raw := scan{kind: scanRaw, y: d.y, xs: d.xs, x0: d.x0, dx: d.dx, n: len(d.y)}
	if d.xs != nil {
		return raw
	}
	from, to := d.indexAt(vx.Lo), d.indexAt(vx.Hi)
	perPixel := float64(max(to-from, 1)) / float64(max(px, 1))
	if logX && vx.Lo > 0 && d.dx > 0 {
		decades := math.Log10(vx.Hi) - math.Log10(vx.Lo)
		if decades > 0 {
			// A pixel column at the right of a log axis covers many more
			// samples than one at the left. A level for the average leaves
			// too many buckets on the right; one for the densest column
			// loses detail on the left. Balance the two in log space, so
			// the scan stays O(width) at both ends.
			dense := vx.Hi * math.Ln10 * decades / (float64(max(px, 1)) * d.dx)
			perPixel = math.Sqrt(perPixel * max(perPixel, dense))
		}
	}
	level := levelFor(perPixel)
	if level < 0 || level >= len(d.py.levels) {
		return raw
	}
	return scan{kind: scanBuckets, buckets: d.py.levels[level], shift: bucketShifts[level],
		x0: d.x0, dx: d.dx, n: len(d.py.levels[level])}
}
