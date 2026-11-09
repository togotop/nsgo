package core

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The commit's benchmarks, NSH's bench_view_tick, bench_high_frequency
// and list scrolling, on the approximate text engine:
//
//	go tool nsgo test -bench . ./core -- -benchmem

// benchTree mounts app in a size surface and commits it once, without
// the tests' check of every commit.
func benchTree(b *testing.B, size Size, app func(cx *Scope) Node) *Tree {
	root := NewRoot(RootOptions{Post: func(func()) {}, Clock: NewManualClock(time.Unix(0, 0)), RequestFrame: func() {}})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Resize(size, Geometry{})
	tree.Mount(app)
	tree.Commit()
	return tree
}

// commitEach runs change, then a commit, b.N times, reporting the mount
// items per commit.
func commitEach(b *testing.B, tree *Tree, change func(i int)) {
	b.Helper()
	items := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		change(i)
		if batch := tree.Commit(); batch != nil {
			items += len(batch.Items)
		}
	}
	b.ReportMetric(float64(items)/float64(b.N), "items/op")
}

// BenchmarkViewTick is a view of 300 static rows and a counter text, the
// counter changing on each commit.
func BenchmarkViewTick(b *testing.B) {
	var tick *Signal[int]
	tree := benchTree(b, Size{Width: 400, Height: 800}, func(cx *Scope) Node {
		tick = NewSignal(cx, 0)
		kids := []Node{Text{ContentFunc: func() string { return fmt.Sprintf("tick %d", tick.Get()) }}}
		for i := range 300 {
			kids = append(kids, View{Style: Style{Direction: Row, Padding: All(8), Gap: 8}, Children: []Node{
				View{Style: Style{Width: Pt(20), Height: Pt(20), Background: RGB(0x3366ff)}},
				Text{Content: fmt.Sprintf("row %d", i), Style: Style{FontSize: 14}},
			}})
		}
		return View{Children: kids}
	})
	commitEach(b, tree, func(i int) { tick.Set(i + 1) })
}

// board is NSH's quote board: 80 rows of a mark and five cells, the
// first cells of 40 rows changing together on each tick. Fixed cells
// have a size of their own; widening values change width every tick.
func board(fixed, widening bool) (func(cx *Scope) Node, func(i int)) {
	const rows, ticking = 80, 40
	var ticks []*Signal[int]
	var root *Scope
	cell := func(content Text) Node {
		s := Style{Padding: All(4)}
		if fixed {
			s.Width, s.Height = Pt(56), Pt(24)
		}
		return View{Style: s, Children: []Node{content}}
	}
	app := func(cx *Scope) Node {
		ticks, root = nil, cx
		var kids []Node
		for row := range rows {
			var first Text
			if row < ticking {
				tick := NewSignal(cx, 0)
				ticks = append(ticks, tick)
				first = Text{ContentFunc: func() string {
					t := tick.Get()
					sign := ""
					if widening && t%2 == 1 {
						sign = "+"
					}
					return fmt.Sprintf("%s%d.%02d", sign, 9+t/3, t%100)
				}}
			} else {
				first = Text{Content: fmt.Sprintf("%d.00", row)}
			}
			cells := []Node{View{Style: Style{Width: Pt(12), Height: Pt(12), Background: RGB(0x22aa66)}}, cell(first)}
			for col := 1; col < 5; col++ {
				cells = append(cells, cell(Text{Content: fmt.Sprintf("r%d c%d", row, col)}))
			}
			kids = append(kids, View{Style: Style{Direction: Row, AlignItems: AlignCenter, Padding: All(4), Gap: 8},
				Children: cells})
		}
		return View{Children: kids}
	}
	change := func(int) {
		root.Batch(func() {
			for _, t := range ticks {
				t.Set(t.GetUntracked() + 7)
			}
		})
	}
	return app, change
}

func BenchmarkBoard(b *testing.B) {
	for _, c := range []struct {
		name            string
		fixed, widening bool
	}{{"fixed", true, false}, {"content", false, false}, {"widening", false, true}} {
		b.Run(c.name, func(b *testing.B) {
			app, change := board(c.fixed, c.widening)
			commitEach(b, benchTree(b, Size{Width: 600, Height: 2400}, app), change)
		})
	}
}

// BenchmarkListScroll scrolls a list of 1000 rows of an avatar, two texts
// and a bound color 10 points a commit.
func BenchmarkListScroll(b *testing.B) {
	var shade *Signal[int]
	tree := benchTree(b, Size{Width: 390, Height: 844}, func(cx *Scope) Node {
		shade = NewSignal(cx, 0)
		return List{Count: func() int { return 1000 }, EstimatedSize: 76, Style: Style{FlexGrow: 1},
			Row: func(cx *Scope, r ListRow) Node {
				return View{Style: Style{Direction: Row, Padding: All(12), Gap: 12, Height: Pt(76)}, Children: []Node{
					View{StyleFunc: func() Style {
						return Style{Width: Pt(52), Height: Pt(52), CornerRadius: 26, Background: RGB(uint32(0x204080 + shade.Get() + r.Index()))}
					}},
					View{Style: Style{FlexGrow: 1}, Children: []Node{
						Text{ContentFunc: func() string { return fmt.Sprintf("Row %d", r.Index()) }, Style: Style{FontSize: 16}},
						Text{ContentFunc: func() string { return fmt.Sprintf("Subtitle of row %d", r.Index()) }, Style: Style{FontSize: 13}},
					}},
				}}
			}}
	})
	var list *node
	for _, n := range tree.nodes {
		if n != nil && n.list != nil && n.scroll != nil {
			list = n
		}
	}
	y := Points(0)
	commitEach(b, tree, func(int) {
		y += 10
		if y > 60000 {
			y = 0
		}
		tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: list.id, Point: Point{Y: y}})
	})
}

// columnPairs reduces ys to the lowest and highest sample of each of width
// pixel columns, in sample order, as a waveform's line keeps the peaks a
// column covers. It reuses out, so a steady frame allocates nothing.
func columnPairs(ys []float32, width int, out []float32) []float32 {
	out = out[:0]
	per := float64(len(ys)) / float64(width)
	for c := range width {
		from, to := int(float64(c)*per), int(float64(c+1)*per)
		if to > len(ys) {
			to = len(ys)
		}
		if from >= to {
			continue
		}
		lo, hi := from, from
		for i := from + 1; i < to; i++ {
			if ys[i] < ys[lo] {
				lo = i
			}
			if ys[i] > ys[hi] {
				hi = i
			}
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		out = append(out, float32(c), ys[lo])
		if hi != lo {
			out = append(out, float32(c), ys[hi])
		}
	}
	return out
}

// benchShifts are the bucket sizes of a pyramid: 64, 256, 1024, ... samples,
// four times the one before, as mygogo's plot keeps across views.
var benchShifts = [...]uint{6, 8, 10, 12, 14, 16}

// benchPyramid holds the extremes of buckets of samples, so a frame scans
// O(width) buckets instead of O(samples) values. A waveform view builds it
// once for a column of data and keeps it across frames.
type benchPyramid struct {
	lo, hi [][]float32
}

// benchLevel is the level whose buckets are no wider than the samples one
// pixel column covers, or -1 to walk the raw samples.
func benchLevel(perPixel float64) int {
	k := -1
	for i, sh := range benchShifts {
		if float64(uint64(1)<<sh) <= perPixel {
			k = i
		}
	}
	return k
}

func (p *benchPyramid) build(ys []float32, level int) {
	p.lo, p.hi = p.lo[:0], p.hi[:0]
	for k := 0; k <= level; k++ {
		p.lo = append(p.lo, nil)
		p.hi = append(p.hi, nil)
	}
	sh := benchShifts[0]
	n := (len(ys) + (1 << sh) - 1) >> sh
	lo, hi := make([]float32, n), make([]float32, n)
	for b := range n {
		l, h := float32(math.Inf(1)), float32(math.Inf(-1))
		for i := b << sh; i < min((b+1)<<sh, len(ys)); i++ {
			l, h = min(l, ys[i]), max(h, ys[i])
		}
		lo[b], hi[b] = l, h
	}
	p.lo[0], p.hi[0] = lo, hi
	for k := 1; k <= level; k++ {
		prevLo, prevHi := p.lo[k-1], p.hi[k-1]
		m := (len(prevLo) + 3) >> 2
		cl, ch := make([]float32, m), make([]float32, m)
		for b := range m {
			l, h := float32(math.Inf(1)), float32(math.Inf(-1))
			for j := b << 2; j < min((b+1)<<2, len(prevLo)); j++ {
				l, h = min(l, prevLo[j]), max(h, prevHi[j])
			}
			cl[b], ch[b] = l, h
		}
		p.lo[k], p.hi[k] = cl, ch
	}
}

// benchBucketPairs reduces a pyramid level to the extremes of each pixel
// column, in bucket order. It reuses out, as columnPairs does.
func benchBucketPairs(p *benchPyramid, level, width int, out []float32) []float32 {
	out = out[:0]
	lo, hi := p.lo[level], p.hi[level]
	scale := float64(width) / float64(len(lo))
	col := func(i int) int { return int(float64(i) * scale) }
	for i := 0; i < len(lo); {
		c := col(i)
		lowV, highV := lo[i], hi[i]
		lowI, highI := i, i
		j := i + 1
		for j < len(lo) && col(j) == c {
			if lo[j] < lowV {
				lowV, lowI = lo[j], j
			}
			if hi[j] > highV {
				highV, highI = hi[j], j
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

// BenchmarkWaveform records a live waveform on every commit, as a plot fed
// by a device would. "raw" gives the canvas every sample, the way a line
// mark traces one point per row; "columns" reduces them to the extremes of
// each pixel column; "pyramid" scans a level of buckets instead, so neither
// the scan nor the recording grows with the number of samples. The "scan"
// cases measure the reduction alone, with no canvas and no commit.
//
//	go tool nsgo test -bench Waveform ./core -- -benchmem
func BenchmarkWaveform(b *testing.B) {
	const (
		width      = 1024
		height     = 400
		maxSamples = 1000000
	)
	// One frame of samples, reused across commits: two tones whose extremes
	// differ inside a column, so the decimation emits both points of it.
	ys := make([]float32, maxSamples)
	for i := range ys {
		ys[i] = float32(200 + 110*math.Sin(float64(i)/37) + 45*math.Sin(float64(i)/3.1))
	}
	for _, c := range []struct {
		name     string
		n        int
		mode     string // "raw", "columns" or "pyramid"
		scanOnly bool
	}{
		{"scan/100k", 100000, "columns", true},
		{"scan-pyramid/100k", 100000, "pyramid", true},
		{"scan-pyramid/1m", maxSamples, "pyramid", true},
		{"raw/1k", 1000, "raw", false},
		{"raw/10k", 10000, "raw", false},
		{"raw/100k", 100000, "raw", false},
		{"columns/100k", 100000, "columns", false},
		{"pyramid/100k", 100000, "pyramid", false},
		{"pyramid/1m", maxSamples, "pyramid", false},
	} {
		// The pyramid belongs to the data, not to a frame: build it once,
		// outside the timer, as a view does before its steady frames.
		var py benchPyramid
		level := -1
		if c.mode == "pyramid" {
			level = benchLevel(float64(c.n) / float64(width))
			py.build(ys[:c.n], level)
		}
		if c.scanOnly {
			b.Run(c.name, func(b *testing.B) {
				var points []float32
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if c.mode == "pyramid" {
						points = benchBucketPairs(&py, level, width, points)
					} else {
						points = columnPairs(ys[:c.n], width, points)
					}
				}
				b.ReportMetric(float64(len(points)/2), "points/frame")
				if level >= 0 {
					b.ReportMetric(float64(len(py.lo[level])), "buckets/frame")
				}
			})
			continue
		}
		b.Run(c.name, func(b *testing.B) {
			var frame *Signal[int]
			var points []float32
			var ops int
			tree := benchTree(b, Size{Width: width, Height: height}, func(cx *Scope) Node {
				frame = NewSignal(cx, 0)
				return Canvas{
					Style: Style{Width: Pt(width), Height: Pt(height)},
					Draw: func(d *Drawing, _ Size) {
						frame.Get() // a new frame of data invalidates the drawing
						data := ys[:c.n]
						d.BeginPath()
						if c.mode == "raw" {
							dx := float32(width) / float32(len(data))
							d.MoveTo(0, data[0])
							for i := 1; i < len(data); i++ {
								d.LineTo(float32(i)*dx, data[i])
							}
						} else {
							if c.mode == "pyramid" {
								points = benchBucketPairs(&py, level, width, points)
							} else {
								points = columnPairs(data, width, points)
							}
							d.MoveTo(points[0], points[1])
							for i := 2; i < len(points); i += 2 {
								d.LineTo(points[i], points[i+1])
							}
						}
						d.SetStrokeColor(driver.RGB(0x1a7f37)).SetLineWidth(1).Stroke()
						ops = len(d.Ops)
					},
				}
			})
			commitEach(b, tree, func(i int) { frame.Set(i + 1) })
			b.ReportMetric(float64(ops), "drawops/frame")
			if level >= 0 {
				b.ReportMetric(float64(len(py.lo[level])), "buckets/frame")
			}
		})
	}
}
