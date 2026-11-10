package waveform

import "math"

// MeasureKind is one automatic reading of a series over an interval.
type MeasureKind uint8

const (
	// MeasureVpp is the peak-to-peak value, the highest sample less the
	// lowest. It is the zero value.
	MeasureVpp MeasureKind = iota
	// MeasureMax and MeasureMin are the highest and lowest samples.
	MeasureMax
	MeasureMin
	// MeasureMean and MeasureRMS are the average and the root mean square.
	MeasureMean
	MeasureRMS
	// MeasureFrequency and MeasurePeriod are the rate of rising crossings
	// of a signal's middle level, and the time between them, in x units.
	MeasureFrequency
	MeasurePeriod
	// MeasureDuty is the fraction of the interval above the middle level.
	MeasureDuty
	// MeasureRise and MeasureFall are the times a signal takes to go from
	// 10% to 90% of its swing, and back, in x units.
	MeasureRise
	MeasureFall
	// MeasureSpan is the width of the measured interval in x units, and
	// MeasureRate is its reciprocal: the time between two cursors and the
	// rate that time implies.
	MeasureSpan
	MeasureRate
)

// MeasureOver says which interval a reading covers.
type MeasureOver uint8

const (
	// MeasureView reads the samples the view shows. It is the zero value.
	MeasureView MeasureOver = iota
	// MeasureMarkers reads between the two measurement markers, so a
	// dragged out stretch of the waveform is measured, not the window.
	MeasureMarkers
)

// Measurement shows one automatic reading of a series. Label is the app's
// word for it and Format its value; the component holds neither.
type Measurement struct {
	Source int
	Kind   MeasureKind
	Label  string
	Over   MeasureOver
	Format func(float64) string
}

// measureFrom reads m from the points the line itself draws: the extremes
// of each pixel column, at most about two points per pixel whatever the
// source holds. xs and ys are parallel data x and sample values, already
// cut to the interval vx. A reading therefore costs what a frame costs,
// not what the retained history costs, and keeps every peak a column
// covers. Frequency, period, duty and edge times are approximate at that
// density, as they are when a scope shows a long stretch at once.
func measureFrom(m Measurement, xs, ys []float64, vx Range) (float64, bool) {
	if m.Source < 0 {
		return 0, false
	}
	if m.Kind == MeasureSpan || m.Kind == MeasureRate {
		if !vx.Valid() {
			return 0, false
		}
		if m.Kind == MeasureSpan {
			return vx.Span(), true
		}
		return 1 / vx.Span(), true
	}
	if len(ys) == 0 {
		return 0, false
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	sum, sumsq, n := 0.0, 0.0, 0
	for _, v := range ys {
		if v != v {
			continue
		}
		lo, hi = min(lo, v), max(hi, v)
		sum += v
		sumsq += v * v
		n++
	}
	if n == 0 {
		return 0, false
	}
	switch m.Kind {
	case MeasureVpp:
		return hi - lo, true
	case MeasureMax:
		return hi, true
	case MeasureMin:
		return lo, true
	case MeasureMean:
		return sum / float64(n), true
	case MeasureRMS:
		return math.Sqrt(sumsq / float64(n)), true
	}
	level := (lo + hi) / 2
	if m.Kind == MeasureDuty {
		high := 0
		for _, v := range ys {
			if v == v && v >= level {
				high++
			}
		}
		return float64(high) / float64(n), true
	}
	crossings := make([]float64, 0, 8)
	for i := 1; i < len(ys); i++ {
		a, b := ys[i-1], ys[i]
		if a != a || b != b {
			continue
		}
		if a < level && b >= level {
			crossings = append(crossings, xs[i])
		}
	}
	switch m.Kind {
	case MeasurePeriod, MeasureFrequency:
		if len(crossings) < 2 {
			return 0, false
		}
		var total float64
		for i := 1; i < len(crossings); i++ {
			total += crossings[i] - crossings[i-1]
		}
		period := total / float64(len(crossings)-1)
		if period <= 0 {
			return 0, false
		}
		if m.Kind == MeasurePeriod {
			return period, true
		}
		return 1 / period, true
	}
	if len(ys) < 3 {
		return 0, false
	}
	low10, high90 := lo+0.1*(hi-lo), lo+0.9*(hi-lo)
	rising := m.Kind == MeasureRise
	for i := 1; i < len(ys); i++ {
		a, b := ys[i-1], ys[i]
		if a != a || b != b {
			continue
		}
		started := a < low10 && b >= low10
		startedFall := a > high90 && b <= high90
		if (rising && !started) || (!rising && !startedFall) {
			continue
		}
		for j := i; j < len(ys); j++ {
			v := ys[j]
			if v != v {
				continue
			}
			done := rising && v >= high90 || !rising && v <= low10
			if done {
				return xs[j] - xs[i], true
			}
		}
		return 0, false
	}
	return 0, false
}
