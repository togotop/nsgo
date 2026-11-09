// Package waveform draws waveforms: series of samples fitted to a viewport
// and reduced to the extremes of each pixel column, so a line of a million
// samples costs a frame what a line of a thousand does. A Chart is a base
// component: it has no look, and the app gives every part its styles, its
// words through Format, Label and Note, and its samples through a Source
// that any goroutine may push to.
package waveform

import (
	"math"
	"strconv"
)

// Range is a stretch of the x or y axis, in data units.
type Range struct {
	Lo, Hi float64
}

// Span is how much of an axis the range covers.
func (r Range) Span() float64 { return r.Hi - r.Lo }

// Valid reports whether the range has a direction: Hi above Lo.
func (r Range) Valid() bool { return r.Hi > r.Lo }

// Viewport is the range along x a Chart shows. A zero X fits the data.
type Viewport struct {
	X Range
}

// ZoomX scales the view around the x at, f < 1 zooming in. It never turns
// the range inside out or empty.
func (v Viewport) ZoomX(f, at float64) Viewport {
	if f <= 0 || !v.X.Valid() {
		return v
	}
	lo := at - (at-v.X.Lo)*f
	hi := at + (v.X.Hi-at)*f
	if hi-lo < 1e-12 {
		return v
	}
	v.X = Range{lo, hi}
	return v
}

// PanX moves the view by dx data units.
func (v Viewport) PanX(dx float64) Viewport {
	if !v.X.Valid() {
		return v
	}
	v.X = Range{v.X.Lo + dx, v.X.Hi + dx}
	return v
}

// Clamp keeps the view within half a screen of empty space beyond each end
// of the data, as panning past an edge settles.
func (v Viewport) Clamp(data Range) Viewport {
	if !v.X.Valid() || !data.Valid() {
		return v
	}
	span := v.X.Span()
	if span >= data.Span() {
		v.X = data
		return v
	}
	lo := min(max(v.X.Lo, data.Lo-span/2), data.Hi+span/2-span)
	v.X = Range{lo, lo + span}
	return v
}

// Fit returns the view of the whole data range.
func (v Viewport) Fit(data Range) Viewport {
	if data.Valid() {
		v.X = data
	}
	return v
}

// Tick is a labelled value on an axis. At is where it lies as a fraction of
// the axis, 0 at its start and 1 at its end; Value is it in data units.
type Tick struct {
	At    float64
	Value float64
	Label string
}

// Axis is one of a Chart's y scales. A zero Range fits it to the series on
// it. Log makes it logarithmic, for values that span decades.
type Axis struct {
	Range  Range
	Log    bool
	Title  string
	Format func(float64) string
}

// ticks returns the axis' ticks over lo..hi, rounded to nice steps.
func (a Axis) ticks(lo, hi float64, count int) []Tick {
	if !(Range{lo, hi}).Valid() || count < 1 {
		return nil
	}
	if a.Log {
		return a.logTicks(lo, hi, count)
	}
	return linearTicks(lo, hi, count, a.Format)
}

func (a Axis) logTicks(lo, hi float64, count int) []Tick {
	first, last := math.Floor(math.Log10(lo)), math.Ceil(math.Log10(hi))
	if last-first > float64(count)*2 {
		// One tick per other decade, so a wide domain stays readable.
		var out []Tick
		for k := first; k <= last; k += 2 {
			v := math.Pow(10, k)
			if v < lo || v > hi {
				continue
			}
			out = append(out, Tick{At: (math.Log10(v) - math.Log10(lo)) / (math.Log10(hi) - math.Log10(lo)),
				Value: v, Label: formatValue(v, a.Format)})
		}
		return out
	}
	var out []Tick
	for k := first; k <= last && len(out) < 64; k++ {
		v := math.Pow(10, k)
		out = append(out, Tick{At: (math.Log10(v) - math.Log10(lo)) / (math.Log10(hi) - math.Log10(lo)),
			Value: v, Label: formatValue(v, a.Format)})
	}
	return out
}

// linearTicks places ticks at multiples of a nice step, at most count of
// them, as d3's ticks and mygogo's chart.NewLinear do.
func linearTicks(lo, hi float64, count int, format func(float64) string) []Tick {
	step := niceStep(hi-lo, count)
	first := math.Ceil(lo/step) * step
	var out []Tick
	for v := first; v <= hi+step*1e-9 && len(out) < 64; v += step {
		if v > math.MaxFloat64-step {
			break
		}
		out = append(out, Tick{At: (v - lo) / (hi - lo), Value: v, Label: formatValue(v, format)})
	}
	return out
}

// niceStep rounds a raw step up to 1, 2, 2.5, 5 or 10 times a power of ten.
func niceStep(span float64, count int) float64 {
	raw := span / float64(max(count, 1))
	if raw <= 0 || math.IsNaN(raw) {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range [...]float64{1, 2, 2.5, 5, 10} {
		if m*mag >= raw {
			return m * mag
		}
	}
	return 10 * mag
}

// timeSteps are the round intervals, in seconds, a time axis picks from.
var timeSteps = [...]float64{
	1e-9, 1e-6, 1e-3, 1, 5, 10, 30, 60, 300, 900, 3600, 6 * 3600, 12 * 3600, 86400,
}

// timeTicks places ticks at round intervals of Unix seconds. Format is the
// app's: the component holds no words for a date or a time.
func timeTicks(lo, hi float64, count int, format func(float64) string) []Tick {
	span := hi - lo
	if span <= 0 || count < 1 {
		return nil
	}
	step := timeSteps[len(timeSteps)-1]
	for _, s := range timeSteps {
		if span/s <= float64(count) {
			step = s
			break
		}
	}
	first := math.Ceil(lo/step) * step
	var out []Tick
	for v := first; v <= hi+step*1e-9 && len(out) < 64; v += step {
		out = append(out, Tick{At: (v - lo) / span, Value: v, Label: formatValue(v, format)})
	}
	return out
}

// formatValue is what a tick shows as text. Without the app's formatter a
// number formats compactly; the app's words go through Format.
func formatValue(v float64, format func(float64) string) string {
	if format != nil {
		return format(v)
	}
	return strconv.FormatFloat(v, 'g', 4, 64)
}

// fitRange returns the y range of an axis from the extremes of the samples
// visible on it: fixed when the axis says so, fitted and rounded otherwise.
func fitRange(a Axis, lo, hi float64) Range {
	if a.Range.Valid() {
		return a.Range
	}
	if hi < lo {
		if a.Log {
			return Range{1, 10}
		}
		return Range{0, 1}
	}
	if a.Log {
		if lo <= 0 {
			lo = hi / 1000
		}
		if lo <= 0 {
			lo = 1
		}
		if hi <= lo {
			hi = lo * 10
		}
		return Range{math.Pow(10, math.Floor(math.Log10(lo))), math.Pow(10, math.Ceil(math.Log10(hi)))}
	}
	span := hi - lo
	if span == 0 {
		span = math.Abs(hi)*0.04 + 1e-9
	}
	step := niceStep(span*1.08, 4)
	return Range{math.Floor((lo - 0.04*span) / step) * step,
		math.Ceil((hi + 0.04*span) / step) * step}
}
