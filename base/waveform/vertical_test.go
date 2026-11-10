package waveform

import "testing"

func TestSeriesVerticalTransform(t *testing.T) {
	s := Series{Scale: 2, Offset: 10}
	if got := s.transform(5, 0); got != 20 {
		t.Fatalf("scale and offset: %v, want 20", got)
	}
	s.Invert = true
	if got := s.transform(5, 0); got != 0 {
		t.Fatalf("invert: %v, want 0", got)
	}
	if got := (Series{}).scaleY(); got != 1 {
		t.Fatalf("a zero scale is %v, want 1", got)
	}
	if got := (Series{AC: true}).transform(5, 5); got != 0 {
		t.Fatalf("AC coupling: %v, want 0", got)
	}
}

func TestWindowWidth(t *testing.T) {
	if got := (Chart{Follow: 2}).windowWidth(Viewport{}, Range{0, 100}); got != 2 {
		t.Fatalf("follow: %v, want 2", got)
	}
	if got := (Chart{TimeBase: 0.5, DivisionsX: 10}).windowWidth(Viewport{}, Range{0, 100}); got != 5 {
		t.Fatalf("time base: %v, want 5", got)
	}
	if got := (Chart{TimeBase: 0.5}).windowWidth(Viewport{}, Range{0, 100}); got != 5 {
		t.Fatalf("time base's default ten columns: %v, want 5", got)
	}
}
