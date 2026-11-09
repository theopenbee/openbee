package kit

import (
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

func TestBeeMarkPicksSmallUnder24(t *testing.T) {
	for _, tk := range []theme.Tokens{theme.Light(), theme.Dark()} {
		if beeMark(tk, 22) != icons.LogoMarkSmall(tk) {
			t.Errorf("beeMark(22) (dark=%v) is not the small mark", tk.Dark)
		}
		if beeMark(tk, 24) != icons.LogoMark(tk) {
			t.Errorf("beeMark(24) (dark=%v) is not the standard mark", tk.Dark)
		}
	}
}

func TestBeeAvatarSizesPickTheSpecVariant(t *testing.T) {
	small := map[AvatarSize]bool{AvatarSM: true, AvatarMD: true, AvatarLG: false, AvatarXL: false}
	for size, wantSmall := range small {
		inner := size.diameter() - 2*(size.beePadding()+1)
		if got := inner < 24; got != wantSmall {
			t.Errorf("size %v draws the mark at %vpx, small=%v, want %v", size, inner, got, wantSmall)
		}
	}
}

func TestDarkBeeAvatarMatchesSurfaceMuted(t *testing.T) {
	tk := theme.Dark()
	if tk.AvatarBee != tk.SurfaceMuted {
		t.Errorf("dark AvatarBee = %v, want SurfaceMuted %v", tk.AvatarBee, tk.SurfaceMuted)
	}
}
