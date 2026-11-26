package model

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	avatarColors = 8
	fnvOffset    = 0x811c9dc5
	fnvPrime     = 0x01000193
	cjkFirst     = 0x4e00
	cjkLast      = 0x9fff
)

func AvatarIndex(id string) int {
	hash := uint32(fnvOffset)
	for _, unit := range utf16.Encode([]rune(id)) {
		hash ^= uint32(unit)
		hash *= fnvPrime
	}
	return int(hash % avatarColors)
}

func Initials(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "?"
	}
	runes := []rune(trimmed)
	if runes[0] >= cjkFirst && runes[0] <= cjkLast {
		return string(runes[0])
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return unicode.IsSpace(r) || r == '_' || r == '-'
	})
	if len(parts) >= 2 {
		return strings.ToUpper(string([]rune(parts[0])[0]) + string([]rune(parts[1])[0]))
	}
	return strings.ToUpper(string(runes[:min(2, len(runes))]))
}

type TileKind int

const (
	TileAvatar TileKind = iota
	TileInitials
	TileRandom
)

type PickerTile struct {
	Kind TileKind
	Ref  string
}

type PickerRow struct {
	Heading bool
	Pack    int
	Tiles   []PickerTile
}

func AvatarLabel(ref string) string {
	if _, name, found := strings.Cut(ref, "/"); found {
		return name
	}
	return ref
}

func FilledPacks(packs []api.AvatarPack) []api.AvatarPack {
	var out []api.AvatarPack
	for _, p := range packs {
		if len(p.Avatars) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func UsedAvatars(workers []api.Worker, except string) map[string]bool {
	used := map[string]bool{}
	for _, w := range workers {
		if w.ID != except && w.Avatar != "" {
			used[w.Avatar] = true
		}
	}
	return used
}

func RandomAvatar(packs []api.AvatarPack, used map[string]bool, intn func(n int) int) string {
	var all, free []string
	for _, p := range packs {
		for _, ref := range p.Avatars {
			all = append(all, ref)
			if !used[ref] {
				free = append(free, ref)
			}
		}
	}
	pool := free
	if len(pool) == 0 {
		pool = all
	}
	if len(pool) == 0 {
		return ""
	}
	return pool[min(max(intn(len(pool)), 0), len(pool)-1)]
}

func PickerColumns(width, tile, gap float32) int {
	if tile <= 0 {
		return 1
	}
	return max(1, int((width+gap)/(tile+gap)))
}

func PickerRows(packs []api.AvatarPack, cols int) []PickerRow {
	cols = max(1, cols)
	choices := []PickerTile{{Kind: TileInitials}, {Kind: TileRandom}}
	if len(packs) == 1 {
		return chunkTiles(nil, 0, append(choices, avatarTiles(packs[0].Avatars)...), cols)
	}
	rows := chunkTiles(nil, 0, choices, cols)
	for i, p := range packs {
		rows = append(rows, PickerRow{Heading: true, Pack: i})
		rows = chunkTiles(rows, i, avatarTiles(p.Avatars), cols)
	}
	return rows
}

func avatarTiles(refs []string) []PickerTile {
	tiles := make([]PickerTile, len(refs))
	for i, ref := range refs {
		tiles[i] = PickerTile{Ref: ref}
	}
	return tiles
}

func chunkTiles(rows []PickerRow, pack int, tiles []PickerTile, cols int) []PickerRow {
	for start := 0; start < len(tiles); start += cols {
		rows = append(rows, PickerRow{Pack: pack, Tiles: tiles[start:min(start+cols, len(tiles))]})
	}
	return rows
}

func PickerRowOf(rows []PickerRow, ref string) int {
	want := PickerTile{Ref: ref}
	if ref == "" {
		want.Kind = TileInitials
	}
	for i, row := range rows {
		if !row.Heading && slices.Contains(row.Tiles, want) {
			return i
		}
	}
	return -1
}
