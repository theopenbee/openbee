package chat

import "math"

const (
	minZoom        = 0.1
	maxZoom        = 8
	longImageRatio = 2
	zoomEpsilon    = 1e-3
	wheelNotch     = 1.25
	pinchWheelMax  = 50
	pinchWheelUnit = 100
)

var zoomSteps = []float32{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8}

func clampZoom(v, lo, hi float32) float32 {
	return max(lo, min(hi, v))
}

func sameZoom(a, b float32) bool {
	return math.Abs(float64(a-b)) < zoomEpsilon
}

func isLongImage(iw, ih float32) bool {
	return ih > longImageRatio*iw
}

func containScale(iw, ih, bw, bh float32) float32 {
	return min(1, bw/iw, bh/ih)
}

func fitScale(iw, ih, bw, bh float32) float32 {
	if isLongImage(iw, ih) {
		return min(1, bw/iw)
	}
	return containScale(iw, ih, bw, bh)
}

func stepZoom(scale float32, up bool, lo, hi float32) float32 {
	next := hi
	if !up {
		next = lo
	}
	for i := range zoomSteps {
		if up && zoomSteps[i] > scale+zoomEpsilon {
			next = zoomSteps[i]
			break
		}
		if s := zoomSteps[len(zoomSteps)-1-i]; !up && s < scale-zoomEpsilon {
			next = s
			break
		}
	}
	return clampZoom(next, lo, hi)
}

func toggleZoom(scale, fit float32) float32 {
	switch {
	case !sameZoom(scale, fit):
		return fit
	case fit < 1:
		return 1
	}
	return 2
}

func wheelZoomFactor(dy float32, precise bool) float32 {
	if precise && math.Abs(float64(dy)) < pinchWheelMax {
		return float32(math.Exp(float64(-dy / pinchWheelUnit)))
	}
	if dy < 0 {
		return wheelNotch
	}
	return 1 / wheelNotch
}

type zoomStage struct {
	w, h, pad, bottom, border float32
	iw, ih                    float32
}

func (s zoomStage) ready() bool {
	return s.iw > 0 && s.ih > 0
}

func (s zoomStage) box() (bw, bh float32) {
	return max(1, s.w-2*s.pad-2*s.border), max(1, s.h-s.bottom-2*s.border)
}

func (s zoomStage) outer(scale float32) (ow, oh float32) {
	return s.iw*scale + 2*s.border, s.ih*scale + 2*s.border
}

func (s zoomStage) fit() float32 {
	bw, bh := s.box()
	return fitScale(s.iw, s.ih, bw, bh)
}

func (s zoomStage) bounds() (lo, hi float32) {
	bw, bh := s.box()
	return min(minZoom, containScale(s.iw, s.ih, bw, bh)), maxZoom
}

func (s zoomStage) origin(scale float32) (x, y float32) {
	ow, oh := s.outer(scale)
	return s.pad + max(0, (s.w-2*s.pad-ow)/2), max(0, (s.h-s.bottom-oh)/2)
}

func (s zoomStage) content(scale float32) (cw, ch float32) {
	ow, oh := s.outer(scale)
	return max(s.w, ow+2*s.pad), max(s.h, oh+s.bottom)
}

func (s zoomStage) overflows(scale float32) bool {
	cw, ch := s.content(scale)
	return cw > s.w+zoomEpsilon || ch > s.h+zoomEpsilon
}

func (s zoomStage) onImage(scale, x, y float32) bool {
	ox, oy := s.origin(scale)
	ow, oh := s.outer(scale)
	return x >= ox && x < ox+ow && y >= oy && y < oy+oh
}

func (s zoomStage) clampScroll(scale, x, y float32) (float32, float32) {
	cw, ch := s.content(scale)
	return max(0, min(x, cw-s.w)), max(0, min(y, ch-s.h))
}

func (s zoomStage) rescroll(from, to, px, py, sx, sy float32) (float32, float32) {
	ox, oy := s.origin(from)
	nx, ny := s.origin(to)
	ox, oy, nx, ny = ox+s.border, oy+s.border, nx+s.border, ny+s.border
	u, v := (sx+px-ox)/from, (sy+py-oy)/from
	return s.clampScroll(to, u*to+nx-px, v*to+ny-py)
}
