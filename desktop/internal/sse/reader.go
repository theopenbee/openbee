package sse

import (
	"bufio"
	"io"
	"strings"
)

const maxLineBytes = 1 << 20

type Event struct{ Data string }

func Read(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	var data []string
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" {
			if data != nil {
				ev := Event{Data: strings.Join(data, "\n")}
				data = nil
				if err := fn(ev); err != nil {
					return err
				}
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		if field != "data" {
			continue
		}
		data = append(data, strings.TrimPrefix(value, " "))
	}
	return sc.Err()
}
