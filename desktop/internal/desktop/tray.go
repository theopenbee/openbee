package desktop

import (
	"log"
	"math"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	windows           = "windows"
	trayID            = "openbee"
	menuBarIconSize   = 36
	trayIconPoints    = 16
	overlayIconPoints = 16
)

type Tray struct {
	goos   string
	native *mygo.Tray
	unread int
	badge  string
}

func showsTray(goos string) bool {
	return goos == darwin || goos == windows
}

func closesToTray(goos string, tray, quitting bool) bool {
	return goos == windows && tray && !quitting
}

func iconPixels(points int, scale float64) int {
	if scale <= 0 {
		scale = 1
	}
	return int(math.Round(float64(points) * scale))
}

func displayScale() float64 {
	return mygo.Screen.PrimaryDisplay().ScaleFactor
}

func TrayToolTip(unread int) string {
	if unread <= 0 {
		return appTitle
	}
	return locales.TrayUnread(unread)
}

func TrayMenu(open func()) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: locales.TrayOpen(), Click: click(open)},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}
}

func NewTray(goos string, open func()) (*Tray, error) {
	t := &Tray{goos: goos}
	icon, template := t.icon("")
	native, err := mygo.NewTray(mygo.TrayOptions{
		ID:             trayID,
		Icon:           icon,
		IconIsTemplate: template,
		ToolTip:        TrayToolTip(0),
	})
	if err != nil {
		return nil, err
	}
	t.native = native
	native.OnClick(open)
	native.OnRightClick(func() {
		native.SetMenu(mygo.NewMenu(TrayMenu(open)))
		native.PopUpMenu()
		native.SetMenu(nil)
	})
	return t, nil
}

func (t *Tray) icon(badge string) ([]byte, bool) {
	if t.goos == darwin {
		return icons.MenuBarIcon(menuBarIconSize), true
	}
	return icons.AppIcon(iconPixels(trayIconPoints, displayScale()), badge), false
}

func (t *Tray) SetUnread(n int) {
	t.unread = n
	t.native.SetToolTip(TrayToolTip(n))
	if t.goos == darwin {
		t.native.SetTitle(model.BadgeText(n))
		return
	}
	badge := model.IconBadgeText(n)
	if badge == t.badge {
		return
	}
	icon, template := t.icon(badge)
	if err := t.native.SetIcon(icon, template); err != nil {
		log.Printf("desktop: tray icon: %v", err)
		return
	}
	t.badge = badge
}

func (t *Tray) Relabel() {
	t.native.SetToolTip(TrayToolTip(t.unread))
}

func (t *Tray) Destroy() {
	t.native.Destroy()
}
