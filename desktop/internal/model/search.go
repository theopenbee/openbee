package model

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	scoreLabelStart  = 100
	scoreLabelWord   = 80
	scoreLabelInside = 50
	scoreKeyword     = 40
	scoreDescription = 25
	scoreLoose       = 10
	insideMinRunes   = 3
	looseMinRunes    = 2
	looseSpread      = 3
)

type Searchable struct {
	Label       string
	Description string
	Keywords    []string
}

func RankSearch(items []Searchable, query string) []int {
	words := strings.Fields(strings.ToLower(query))
	type ranked struct{ index, score int }
	out := make([]ranked, 0, len(items))
	for i, item := range items {
		if score, ok := scoreItem(item, words); ok {
			out = append(out, ranked{i, score})
		}
	}
	slices.SortStableFunc(out, func(a, b ranked) int { return b.score - a.score })
	order := make([]int, len(out))
	for i, r := range out {
		order[i] = r.index
	}
	return order
}

func scoreItem(item Searchable, words []string) (int, bool) {
	label := strings.ToLower(item.Label)
	total := 0
	for _, word := range words {
		score := scoreWord(item, label, word)
		if score == 0 {
			return 0, false
		}
		total += score
	}
	return total, true
}

func scoreWord(item Searchable, label, word string) int {
	switch at := wordStart(label, word); {
	case at == 0:
		return scoreLabelStart
	case at > 0:
		return scoreLabelWord
	}
	if utf8.RuneCountInString(word) >= insideMinRunes && strings.Contains(label, word) {
		return scoreLabelInside
	}
	for _, keyword := range item.Keywords {
		if wordStart(strings.ToLower(keyword), word) >= 0 {
			return scoreKeyword
		}
	}
	if wordStart(strings.ToLower(item.Description), word) >= 0 {
		return scoreDescription
	}
	if loose(label, word) {
		return scoreLoose
	}
	return 0
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func wordStart(text, word string) int {
	if word == "" {
		return -1
	}
	for from := 0; from <= len(text); {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return -1
		}
		at := from + i
		if at == 0 {
			return 0
		}
		r, size := utf8.DecodeRuneInString(text[at:])
		prev, _ := utf8.DecodeLastRuneInString(text[:at])
		if isCJK(r) || !isWordRune(prev) {
			return at
		}
		from = at + size
	}
	return -1
}

func loose(text, word string) bool {
	t, w := []rune(text), []rune(word)
	if len(w) < looseMinRunes {
		return false
	}
	for start, r := range t {
		if r != w[0] || start > 0 && isWordRune(t[start-1]) {
			continue
		}
		at, found := start, true
		for _, next := range w[1:] {
			j := slices.Index(t[at+1:], next)
			if j < 0 {
				found = false
				break
			}
			at += j + 1
		}
		if found && at-start < len(w)*looseSpread {
			return true
		}
	}
	return false
}
