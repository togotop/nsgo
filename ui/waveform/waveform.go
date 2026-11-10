// Package waveform gives base's waveform chart a look in ui's theme. The
// component holds no behavior of its own: it is base/waveform.Chart with
// colors, type and the cross line's and markers' looks filled in.
package waveform

import (
	base "github.com/nsgo-dev/nsgo/base/waveform"
	"github.com/nsgo-dev/nsgo/core"
)

// WaveformVariant is a Waveform's look: the color of its grid.
type WaveformVariant uint8

const (
	// WaveformNeutral grids in a neutral gray. It is the default.
	WaveformNeutral WaveformVariant = iota
	// WaveformAccent grids in the accent, for a chart that stands out.
	WaveformAccent
)

// grid is the color the variant draws its grid in, the same in every theme
// so a switch of appearance never records the waveform again.
func (v WaveformVariant) grid() core.Color {
	if v == WaveformAccent {
		return core.RGBA(0x288cf0, 0.45)
	}
	return core.RGBA(0x888888, 0.35)
}

// seriesColors color the series that have no color of their own, all opaque:
// a translucent line falls back to a slower drawing path.
var seriesColors = [...]core.Color{
	core.RGB(0x288cf0), core.RGB(0xf06446), core.RGB(0x32be6e),
	core.RGB(0xc8a028), core.RGB(0x965ac8), core.RGB(0x50b4c8),
}

// Waveform is base/waveform.Chart in the theme's look. Save the chart's
// fields as its own; Variant chooses the grid's color.
type Waveform struct {
	base.Chart
	Variant WaveformVariant
}

// Render implements core.Node. The look is type and a fixed chart palette:
// a chart's series keep their colors in every theme, and the grid, cross
// line and markers are faint enough for both. Text takes the inherited
// color, so a switch of appearance restyles it in place without recording
// the waveform or rebuilding a view.
func (w Waveform) Render(*core.Scope) core.Node {
	c := w.Chart
	if c.GridColor == 0 {
		c.GridColor = w.Variant.grid()
	}
	c.AxisStyle = core.Style{FontSize: 11}.Merge(c.AxisStyle)
	c.LegendStyle = core.Style{FontSize: 11}.Merge(c.LegendStyle)
	c.MeasurementStyle = core.Style{FontSize: 11}.Merge(c.MeasurementStyle)
	if c.TriggerColor == 0 {
		// A fixed chart color, as the grid's is: the trigger line is a
		// recorded drawing operation, not part of the theme's look.
		c.TriggerColor = core.RGBA(0xcc3333, 0.8)
	}
	cross := core.RGBA(0x888888, 0.45)
	if w.Variant == WaveformAccent {
		cross = core.RGBA(0x288cf0, 0.6)
	}
	c.CrossLineStyle = core.Style{Background: cross}.Merge(c.CrossLineStyle)
	c.MarkStyle = core.Style{Background: core.RGBA(0x288cf0, 0.8)}.Merge(c.MarkStyle)
	for i := range c.Series {
		if c.Series[i].Color == 0 {
			c.Series[i].Color = seriesColors[i%len(seriesColors)]
		}
	}
	return c
}
