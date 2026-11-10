package waveform

import (
	"math"
	"sort"
)

const (
	scanRaw uint8 = iota
	scanBuckets
)

// scan walks a series in x order: its samples, or the buckets of a pyramid
// level. columns emits from a scan, so the raw path and the bucketed one
// share one emitter and differ only in what an item covers.
type scan struct {
	kind uint8
	// Raw samples: y and either x0/dx (uniform) or xs (irregular, ascending).
	y      []float32
	x0, dx float64
	xs     []float64
	// A pyramid level's buckets: bucket b starts at x0+(b<<shift)*dx.
	buckets []minmax
	shift   uint
	n       int
}

func (sc scan) x(i int) float64 {
	if sc.kind == scanBuckets {
		return sc.x0 + float64(int64(i)<<sc.shift)*sc.dx
	}
	if sc.xs != nil {
		return sc.xs[i]
	}
	return sc.x0 + float64(i)*sc.dx
}

func (sc scan) low(i int) float32 {
	if sc.kind == scanBuckets {
		return sc.buckets[i].lo
	}
	return sc.y[i]
}

func (sc scan) high(i int) float32 {
	if sc.kind == scanBuckets {
		return sc.buckets[i].hi
	}
	return sc.y[i]
}

// indexAt returns the first item at or after x, clamped to the ends.
func (sc scan) indexAt(x float64) int {
	if sc.n == 0 {
		return 0
	}
	if sc.kind == scanRaw && sc.xs != nil {
		i := sort.Search(sc.n, func(i int) bool { return sc.xs[i] >= x })
		return min(max(i, 0), sc.n-1)
	}
	d := sc.dx
	if sc.kind == scanBuckets {
		d *= float64(int64(1) << sc.shift)
	}
	if d <= 0 {
		return 0
	}
	i := int(math.Ceil((x - sc.x0) / d))
	return min(max(i, 0), sc.n-1)
}

// columns reduces the visible part of sc to the lowest and highest of each
// pixel column of a plot px wide, as flat x,y pairs: x is the column, y a
// value in the data's units. One item beyond each edge lets the line reach
// the border. It appends to out, so a steady frame allocates nothing.
func columns(sc scan, vx Range, px int, logX bool, out []float32) []float32 {
	out = out[:0]
	if sc.n == 0 || px <= 0 || !vx.Valid() {
		return out
	}
	from := max(0, sc.indexAt(vx.Lo)-1)
	to := min(sc.n, sc.indexAt(vx.Hi)+1)
	var col func(i int) int
	if logX && vx.Lo > 0 {
		llo, lhi := math.Log10(vx.Lo), math.Log10(vx.Hi)
		if lhi > llo {
			col = func(i int) int {
				x := sc.x(i)
				if x <= 0 {
					return -1
				}
				return int(math.Floor((math.Log10(x) - llo) / (lhi - llo) * float64(px)))
			}
		}
	}
	if col == nil {
		scale := float64(px) / vx.Span()
		col = func(i int) int { return int(math.Floor((sc.x(i) - vx.Lo) * scale)) }
	}
	// An item on or just past an edge belongs to that edge's column: its
	// extreme is the line's own, and dropping it would lose the highest
	// value the view holds, as a spectrum's first bin often is.
	raw := col
	col = func(i int) int {
		if c := raw(i); c > 0 {
			return min(c, px)
		}
		return 0
	}
	// A NaN item opens a break; the next finite column writes a NaN pair
	// into the output, which the drawing reads as the start of a new
	// segment, so a gap in the data is a gap in the line.
	broken := false
	for i := from; i < to; {
		if sc.low(i) != sc.low(i) {
			broken = true
			i++
			continue
		}
		if broken {
			if len(out) > 0 {
				out = append(out, nan, nan)
			}
			broken = false
		}
		c := col(i)
		lowI, highI := i, i
		lowV, highV := sc.low(i), sc.high(i)
		j := i + 1
		for j < to {
			lj, hj := sc.low(j), sc.high(j)
			if lj != lj || col(j) != c {
				break
			}
			if lj < lowV {
				lowV, lowI = lj, j
			}
			if hj > highV {
				highV, highI = hj, j
			}
			j++
		}
		a, b := lowI, highI
		if a > b {
			a, b = b, a
		}
		out = append(out, float32(c), lowV)
		if b != a {
			out = append(out, float32(c), highV)
		}
		i = j
	}
	return out
}

// segments calls fn for each run of finite points in pts, flat x,y pairs.
// A NaN pair separates two runs, so a break in the data breaks the line
// and the area under it.
func segments(pts []float32, fn func(seg []float32)) {
	for i := 0; i+1 < len(pts); {
		if pts[i] != pts[i] {
			i += 2
			continue
		}
		j := i + 2
		for j+1 < len(pts) && pts[j] == pts[j] {
			j += 2
		}
		fn(pts[i:j])
		i = j
	}
}

// geomKey says when a series' columns have to be reduced again: a new
// view, a new width, or new data. Style changes are not here, as they only
// affect painting.
type geomKey struct {
	lo, hi  float64
	px      int
	version uint64
	n       int
	x0, dx  float64
	logX    bool
}

// geom is one series' reduced points for the last frame of a view.
type geom struct {
	key    geomKey
	points []float32
	ready  bool
}

// get returns the points for key, decimating again only when it changed.
func (g *geom) get(key geomKey, sc scan, vx Range, px int) []float32 {
	if g.ready && g.key == key {
		return g.points
	}
	g.key, g.ready = key, true
	g.points = columns(sc, vx, px, key.logX, g.points)
	return g.points
}
