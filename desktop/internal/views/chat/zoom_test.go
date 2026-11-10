package chat_test

import (
	"math"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/views/chat"
)

const boxW, boxH = 1000, 800

func TestFitScale(t *testing.T) {
	for _, tc := range []struct {
		name   string
		iw, ih float32
		want   float32
	}{
		{"small image stays at full size", 400, 300, 1},
		{"wide image fits the width", 2000, 1000, 0.5},
		{"tall image fits the height", 1000, 1600, 0.5},
		{"exactly twice as tall fits the height", 500, 1000, 0.8},
		{"long screenshot fits the width only", 2000, 8000, 0.5},
		{"narrow long screenshot stays at full size", 500, 8000, 1},
	} {
		if got := chat.FitScale(tc.iw, tc.ih, boxW, boxH); got != tc.want {
			t.Errorf("%s: fitScale(%v×%v) = %v, want %v", tc.name, tc.iw, tc.ih, got, tc.want)
		}
	}
}

func TestStepZoom(t *testing.T) {
	for _, tc := range []struct {
		scale  float32
		up     bool
		lo, hi float32
		want   float32
	}{
		{1, true, 0.1, 8, 1.25},
		{1, false, 0.1, 8, 0.75},
		{0.37, true, 0.1, 8, 0.5},
		{0.37, false, 0.1, 8, 0.25},
		{8, true, 0.1, 8, 8},
		{0.1, false, 0.1, 8, 0.1},
		{0.1, false, 0.05, 8, 0.05},
		{0.05, true, 0.05, 8, 0.1},
	} {
		if got := chat.StepZoom(tc.scale, tc.up, tc.lo, tc.hi); got != tc.want {
			t.Errorf("stepZoom(%v, up=%v, %v..%v) = %v, want %v", tc.scale, tc.up, tc.lo, tc.hi, got, tc.want)
		}
	}
}

func TestToggleZoom(t *testing.T) {
	for _, tc := range []struct{ scale, fit, want float32 }{
		{0.4, 0.4, 1},
		{1, 0.4, 0.4},
		{3, 0.4, 0.4},
		{1, 1, 2},
		{2, 1, 1},
	} {
		if got := chat.ToggleZoom(tc.scale, tc.fit); got != tc.want {
			t.Errorf("toggleZoom(%v, fit %v) = %v, want %v", tc.scale, tc.fit, got, tc.want)
		}
	}
}

func TestWheelZoomFactor(t *testing.T) {
	if got, want := chat.WheelZoomFactor(-10, true), float32(math.Exp(0.1)); math.Abs(float64(got-want)) > 1e-6 {
		t.Errorf("touchpad -10 = %v, want %v", got, want)
	}
	if got := chat.WheelZoomFactor(-100, false); got != 1.25 {
		t.Errorf("wheel notch up = %v, want 1.25", got)
	}
	if got := chat.WheelZoomFactor(120, false); got != 0.8 {
		t.Errorf("wheel notch down = %v, want 0.8", got)
	}
	if got := chat.WheelZoomFactor(-80, true); got != 1.25 {
		t.Errorf("large touchpad step = %v, want a notch of 1.25", got)
	}
}
