package waveform_test

import (
	"os"
	"testing"

	"github.com/nsgo-dev/nsgo/tsx/tsxtest"
	_ "github.com/nsgo-dev/nsgo/tsx/ui"     // registers nsgo/ui's styled waveform
	_ "github.com/nsgo-dev/nsgo/tsx/waveform" // registers nsgo/base's waveform
)

// TestWaveformTSX runs the TSX tests beside this file: a TSX app mounts the
// registered Waveform from both modules, so the bindings are checked from
// the side an app sees them, not only from Go.
func TestWaveformTSX(t *testing.T) {
	tsxtest.Run(t, os.DirFS("testdata"))
}
