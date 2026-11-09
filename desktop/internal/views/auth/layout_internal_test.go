package auth

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
)

func TestLogoAspectMatchesLockup(t *testing.T) {
	data, err := os.ReadFile("../../icons/brand/logo-full.svg")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`viewBox="0 0 ([\d.]+) ([\d.]+)"`).FindSubmatch(data)
	if m == nil {
		t.Fatal("logo-full.svg has no viewBox starting at 0 0")
	}
	w, _ := strconv.ParseFloat(string(m[1]), 64)
	h, _ := strconv.ParseFloat(string(m[2]), 64)
	if math.Abs(w/h-logoAspect) > 1e-6 {
		t.Errorf("logoAspect = %v, lockup viewBox is %vx%v (%v)", logoAspect, w, h, w/h)
	}
}
