package waveform

import "math"

// bucketShifts are the bucket sizes of the levels: 64, 256, 1024, ...
// samples, four times the one before. A frame scans the level whose buckets
// are no wider than the samples one pixel column covers.
var bucketShifts = [...]uint{6, 8, 10, 12, 14, 16, 18}

// minmax are the lowest and highest of a bucket of samples. NaN in both
// says the bucket holds no finite sample, and breaks the line there.
type minmax struct {
	lo, hi float32
}

var nan = float32(math.NaN())

// levelFor returns the level whose buckets are no wider than the samples
// one pixel column covers, or -1 to walk the raw samples.
func levelFor(perPixel float64) int {
	k := -1
	for i, sh := range bucketShifts {
		if float64(int64(1)<<sh) <= perPixel {
			k = i
		}
	}
	return k
}

// pyramid holds the extremes of buckets of samples: level k's bucket b
// covers samples [b<<shift, (b+1)<<shift). It grows as samples append, so
// only the buckets a new batch reaches and their parents are rebuilt; a
// view that moves scans buckets and rebuilds none.
type pyramid struct {
	levels [][]minmax
	n      int
	x0, dx float64
}

// extend updates the levels for samples appended to y, from index from to
// the end. It is the only place a pyramid changes after its first build.
func (p *pyramid) extend(y []float32, from int, x0, dx float64) {
	p.n, p.x0, p.dx = len(y), x0, dx
	if len(y) == 0 {
		p.levels = p.levels[:0]
		return
	}
	if from > len(y) {
		from = len(y)
	}
	if from < 0 {
		from = 0
	}
	b0, b1 := int(from>>bucketShifts[0]), int((len(y)-1)>>bucketShifts[0])
	for l, sh := range bucketShifts {
		if l > 0 {
			b0, b1 = b0>>2, b1>>2
		}
		need := (len(y) + (1 << sh) - 1) >> sh
		if len(p.levels) <= l {
			p.levels = append(p.levels, nil)
		}
		if cap(p.levels[l]) < need {
			// A new backing array must carry the buckets the append does
			// not reach; only from's bucket and those after it are rebuilt.
			grown := make([]minmax, need, max(need*2, 64))
			copy(grown, p.levels[l])
			p.levels[l] = grown
		} else {
			p.levels[l] = p.levels[l][:need]
		}
		if b1 >= need {
			b1 = need - 1
		}
		for b := b0; b <= b1; b++ {
			if l == 0 {
				p.levels[l][b] = extreme(y, b<<sh, min((b+1)<<sh, len(y)))
			} else {
				prev := p.levels[l-1]
				p.levels[l][b] = combine(prev, b<<2, min((b+1)<<2, len(prev)))
			}
		}
		if b1 < 0 {
			break
		}
	}
}

// extreme is the lowest and highest finite sample of y[from:to].
func extreme(y []float32, from, to int) minmax {
	lo, hi := nan, nan
	for i := from; i < to; i++ {
		v := y[i]
		if v != v {
			continue
		}
		if lo != lo || v < lo {
			lo = v
		}
		if hi != hi || v > hi {
			hi = v
		}
	}
	return minmax{lo, hi}
}

// combine is the lowest and highest of buckets[from:to] of a finer level.
func combine(buckets []minmax, from, to int) minmax {
	lo, hi := nan, nan
	for i := from; i < to; i++ {
		b := buckets[i]
		if b.lo != b.lo {
			continue
		}
		if lo != lo || b.lo < lo {
			lo = b.lo
		}
		if hi != hi || b.hi > hi {
			hi = b.hi
		}
	}
	return minmax{lo, hi}
}
