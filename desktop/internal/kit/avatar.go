package kit

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

type AvatarSize int

const (
	AvatarSM AvatarSize = iota
	AvatarMD
	AvatarLG
	AvatarXL
)

const (
	avatarDot         = 10
	avatarDotXL       = 14
	avatarDotOffset   = -2
	avatarDotOffsetXL = 4
	unknownInitials   = "?"
)

func (s AvatarSize) diameter() float32 {
	switch s {
	case AvatarSM:
		return 28
	case AvatarLG:
		return 48
	case AvatarXL:
		return 88
	}
	return 36
}

func (s AvatarSize) fontSize() float32 {
	switch s {
	case AvatarSM:
		return 10
	case AvatarLG:
		return 14
	case AvatarXL:
		return 22
	}
	return 12
}

func (s AvatarSize) beePadding() float32 {
	switch s {
	case AvatarSM:
		return 4
	case AvatarLG:
		return 8
	case AvatarXL:
		return 14
	}
	return 6
}

func (s AvatarSize) dot() (d, offset float32) {
	if s == AvatarXL {
		return avatarDotXL, avatarDotOffsetXL
	}
	return avatarDot, avatarDotOffset
}

func WorkerAvatar(c *ui.Context, tk theme.Tokens, id, name, status string, size AvatarSize, withStatus bool) ui.Element {
	d := size.diameter()
	return ui.Box(c).Size(d, d).Shrink(0).Radius(theme.RadiusPill).Background(tk.Avatars[model.AvatarIndex(id)]).Children(func() {
		avatarFace(c, tk, model.Initials(name), size).Fill().TextColor(tk.AvatarForeground)
		if withStatus {
			dot, offset := size.dot()
			StatusDot(c, tk, status, dot).Absolute().Right(offset).Bottom(offset)
		}
	})
}

func BeeAvatar(c *ui.Context, tk theme.Tokens, size AvatarSize) ui.Element {
	d := size.diameter()
	pad := size.beePadding()
	return ui.Box(c).Size(d, d).Shrink(0).Center().Padding(pad).Radius(theme.RadiusPill).
		Background(tk.AvatarBee).Border(1, tk.Border).Children(func() {
		inner := d - 2*(pad+1)
		ui.Image(c, icons.LogoMark(tk)).Size(inner, inner)
	})
}

func UnknownAvatar(c *ui.Context, tk theme.Tokens, size AvatarSize) ui.Element {
	return InitialsAvatar(c, tk, unknownInitials, size)
}

func InitialsAvatar(c *ui.Context, tk theme.Tokens, name string, size AvatarSize) ui.Element {
	d := size.diameter()
	return avatarFace(c, tk, arcInitials(name), size).Size(d, d).Shrink(0).Background(tk.SurfaceMuted).TextColor(tk.Foreground)
}

func avatarFace(c *ui.Context, tk theme.Tokens, initials string, size AvatarSize) ui.Element {
	return ui.Box(c).Center().Radius(theme.RadiusPill).Border(1, tk.Border).Children(func() {
		ui.Text(c, initials).FontSize(size.fontSize()).FontWeight(mediumWeight).SingleLine()
	})
}

func arcInitials(name string) string {
	var b strings.Builder
	for i, part := range strings.Fields(name) {
		if i == 2 {
			break
		}
		r, _ := utf8.DecodeRuneInString(part)
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
