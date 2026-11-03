package model

import (
	"strings"
	"time"
)

type Relative struct {
	Unit string
	N    int
}

func isZh(lang string) bool {
	return strings.HasPrefix(lang, "zh")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func ListTime(ts int64, now time.Time, lang string) (string, bool) {
	t := time.UnixMilli(ts).In(now.Location())
	if sameDay(t, now) {
		return t.Format("15:04"), false
	}
	if sameDay(t, now.AddDate(0, 0, -1)) {
		return "", true
	}
	if t.Year() == now.Year() {
		return t.Format("1/2"), false
	}
	if isZh(lang) {
		return t.Format("2006/1/2"), false
	}
	return t.Format("1/2/2006"), false
}

func MessageTime(ts int64, now time.Time, lang string) string {
	if ts == 0 {
		return ""
	}
	t := time.UnixMilli(ts).In(now.Location())
	switch {
	case sameDay(t, now):
		return t.Format("15:04")
	case isZh(lang):
		return t.Format("1月2日 15:04")
	}
	return t.Format("Jan 2, 15:04")
}

func RelativeTime(ts int64, now time.Time) Relative {
	if ts == 0 {
		return Relative{}
	}
	seconds := (now.UnixMilli() - ts) / 1000
	if seconds < 60 {
		return Relative{Unit: "now"}
	}
	minutes := seconds / 60
	if minutes < 60 {
		return Relative{Unit: "minutes", N: int(minutes)}
	}
	hours := minutes / 60
	if hours < 24 {
		return Relative{Unit: "hours", N: int(hours)}
	}
	return Relative{Unit: "days", N: int(hours / 24)}
}
