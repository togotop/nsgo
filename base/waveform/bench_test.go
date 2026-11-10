package waveform

import "testing"

// BenchmarkDecimate is the reduction's budget: above a few samples per
// pixel a frame scans a pyramid level at the plot's width, so a million
// samples cost what a hundred thousand do.
func BenchmarkWaveformDecimate(b *testing.B) {
	const px = 1024
	for _, c := range []struct {
		name string
		n    int
	}{
		{"raw/1k", 1000},
		{"buckets/100k", 100000},
		{"buckets/1m", 1000000},
	} {
		b.Run(c.name, func(b *testing.B) {
			d := &data{dx: 0.001}
			d.append(samples(c.n))
			vx := d.extent()
			sc := d.scanFor(vx, px, false)
			var out []float32
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				out = columns(sc, vx, px, false, out)
			}
			b.ReportMetric(float64(len(out)/2), "points/frame")
			if sc.kind == scanBuckets {
				b.ReportMetric(float64(sc.n), "buckets/frame")
			}
		})
	}
}

// BenchmarkAppend is what a device thread's batch costs once the engine
// adopts it: extend rebuilds only the tail buckets the new samples reach,
// on a column already holding a million samples.
func BenchmarkWaveformAppend(b *testing.B) {
	d := &data{dx: 1.0 / 20000}
	d.append(samples(1000000))
	d.append(samples(333))
	from := len(d.y) - 333
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		d.py.extend(d.y, from, d.x0, d.dx)
	}
	b.ReportMetric(float64(len(d.py.levels[0])), "buckets")
}
