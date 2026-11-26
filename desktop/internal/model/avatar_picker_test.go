package model

import (
	"reflect"
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func pack(id string, refs ...string) api.AvatarPack {
	return api.AvatarPack{ID: id, Name: api.LocalizedText{"en": id}, Avatars: refs}
}

func TestAvatarLabel(t *testing.T) {
	for ref, want := range map[string]string{"mfers/00042": "00042", "plain": "plain", "": ""} {
		if got := AvatarLabel(ref); got != want {
			t.Errorf("AvatarLabel(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestFilledPacksDropsEmptyPacks(t *testing.T) {
	got := FilledPacks([]api.AvatarPack{pack("a", "a/1"), pack("empty"), pack("b", "b/1")})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("FilledPacks = %+v, want a and b", got)
	}
	if FilledPacks(nil) != nil {
		t.Fatal("FilledPacks(nil) must be nil")
	}
}

func TestUsedAvatarsSkipsTheWorkerAndBlanks(t *testing.T) {
	workers := []api.Worker{{ID: "w1", Avatar: "a/1"}, {ID: "w2", Avatar: "a/2"}, {ID: "w3"}}
	if got := UsedAvatars(workers, "w1"); !reflect.DeepEqual(got, map[string]bool{"a/2": true}) {
		t.Fatalf("UsedAvatars = %v, want only a/2", got)
	}
}

func TestRandomAvatarPicksAFreeAvatar(t *testing.T) {
	packs := []api.AvatarPack{pack("a", "a/1", "a/2"), pack("b", "b/1")}
	used := map[string]bool{"a/1": true, "b/1": true}
	for n := range 3 {
		if got := RandomAvatar(packs, used, func(int) int { return n }); got != "a/2" {
			t.Fatalf("RandomAvatar(pick %d) = %q, want the only free avatar a/2", n, got)
		}
	}
	var sizes []int
	got := RandomAvatar(packs, map[string]bool{}, func(n int) int { sizes = append(sizes, n); return 2 })
	if got != "b/1" || !slices.Equal(sizes, []int{3}) {
		t.Fatalf("RandomAvatar = %q from pools %v, want b/1 from a pool of 3", got, sizes)
	}
}

func TestRandomAvatarFallsBackToAnyAvatar(t *testing.T) {
	packs := []api.AvatarPack{pack("a", "a/1", "a/2")}
	used := map[string]bool{"a/1": true, "a/2": true}
	if got := RandomAvatar(packs, used, func(int) int { return 1 }); got != "a/2" {
		t.Fatalf("RandomAvatar = %q, want a/2 from all avatars once every one is used", got)
	}
	if got := RandomAvatar(nil, nil, func(int) int { return 0 }); got != "" {
		t.Fatalf("RandomAvatar without packs = %q, want empty", got)
	}
	if got := RandomAvatar(packs, nil, func(int) int { return 99 }); got != "a/2" {
		t.Fatalf("RandomAvatar clamps an out-of-range pick, got %q", got)
	}
}

func TestPickerColumns(t *testing.T) {
	tests := []struct {
		width, tile, gap float32
		want             int
	}{
		{504, 56, 8, 8},
		{503, 56, 8, 7},
		{40, 56, 8, 1},
		{100, 0, 8, 1},
	}
	for _, tc := range tests {
		if got := PickerColumns(tc.width, tc.tile, tc.gap); got != tc.want {
			t.Errorf("PickerColumns(%v, %v, %v) = %d, want %d", tc.width, tc.tile, tc.gap, got, tc.want)
		}
	}
}

func refTiles(refs ...string) []PickerTile {
	return avatarTiles(refs)
}

var (
	initialsTile = PickerTile{Kind: TileInitials}
	randomTile   = PickerTile{Kind: TileRandom}
)

func TestPickerRowsOnePackFlowsAfterTheChoices(t *testing.T) {
	rows := PickerRows([]api.AvatarPack{pack("a", "a/1", "a/2", "a/3", "a/4", "a/5")}, 3)
	want := []PickerRow{
		{Tiles: append([]PickerTile{initialsTile, randomTile}, refTiles("a/1")...)},
		{Tiles: refTiles("a/2", "a/3", "a/4")},
		{Tiles: refTiles("a/5")},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	if got := PickerRowOf(rows, ""); got != 0 {
		t.Fatalf("PickerRowOf(initials) = %d, want 0", got)
	}
	if got := PickerRowOf(rows, "a/5"); got != 2 {
		t.Fatalf("PickerRowOf(a/5) = %d, want 2", got)
	}
}

func TestPickerRowsHeadEachPackWhenSeveral(t *testing.T) {
	rows := PickerRows([]api.AvatarPack{pack("a", "a/1", "a/2", "a/3"), pack("b", "b/1")}, 3)
	want := []PickerRow{
		{Tiles: []PickerTile{initialsTile, randomTile}},
		{Heading: true, Pack: 0},
		{Pack: 0, Tiles: refTiles("a/1", "a/2", "a/3")},
		{Heading: true, Pack: 1},
		{Pack: 1, Tiles: refTiles("b/1")},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	if got := PickerRowOf(rows, "b/1"); got != 4 {
		t.Fatalf("PickerRowOf(b/1) = %d, want 4", got)
	}
	if got := PickerRowOf(rows, "gone/1"); got != -1 {
		t.Fatalf("PickerRowOf(unknown) = %d, want -1", got)
	}
	if got := PickerRows(nil, 0); len(got) != 2 || got[0].Tiles[0] != initialsTile || got[1].Tiles[0] != randomTile {
		t.Fatalf("PickerRows without packs and one column = %+v, want the two choices", got)
	}
}
