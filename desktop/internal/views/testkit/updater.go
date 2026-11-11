package testkit

import (
	"time"

	"github.com/theopenbee/openbee/desktop/internal/store"
)

type Updater struct {
	Checks     int
	Last       time.Time
	Automatic  bool
	Installing bool
}

func (u *Updater) Check()                        { u.Checks++ }
func (u *Updater) LastCheck() time.Time          { return u.Last }
func (u *Updater) AutomaticChecks() bool         { return u.Automatic }
func (u *Updater) SetAutomaticChecks(on bool)    { u.Automatic = on }
func (u *Updater) AutomaticDownloads() bool      { return u.Installing }
func (u *Updater) SetAutomaticDownloads(on bool) { u.Installing = on }

func WithUpdater(u store.Updater, version string) func(o *store.Options) {
	return func(o *store.Options) {
		o.Updater = u
		o.Version = version
	}
}
