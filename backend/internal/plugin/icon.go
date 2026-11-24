package plugin

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const MaxIconSize = 32 << 10

const svgNamespace = "http://www.w3.org/2000/svg"

var forbiddenIconElements = []string{"script", "foreignobject"}

func loadIcon(dir, rel string) (string, error) {
	if !filepath.IsLocal(filepath.FromSlash(rel)) || !strings.EqualFold(filepath.Ext(rel), ".svg") {
		return "", fmt.Errorf("icon %q must be a relative path to an .svg file inside the plugin", rel)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("read icon: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("icon %q is not a regular file", rel)
	}
	if info.Size() > MaxIconSize {
		return "", fmt.Errorf("icon %q is %d bytes, larger than %d", rel, info.Size(), MaxIconSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read icon: %w", err)
	}
	if err := ValidateIcon(data); err != nil {
		return "", fmt.Errorf("icon %q: %w", rel, err)
	}
	return string(data), nil
}

func ValidateIcon(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	seenRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("not valid XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				if seenRoot || name != "svg" {
					return errors.New("root element must be <svg>")
				}
				if t.Name.Space != svgNamespace {
					return fmt.Errorf("<svg> must declare xmlns=%q", svgNamespace)
				}
				seenRoot = true
			}
			if slices.Contains(forbiddenIconElements, name) {
				return fmt.Errorf("<%s> is not allowed", t.Name.Local)
			}
			for _, a := range t.Attr {
				attr := strings.ToLower(a.Name.Local)
				if strings.HasPrefix(attr, "on") {
					return fmt.Errorf("event attribute %q is not allowed", a.Name.Local)
				}
				if attr == "href" && !strings.HasPrefix(strings.TrimSpace(a.Value), "#") {
					return errors.New("links outside the icon are not allowed")
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.Directive:
			return errors.New("DOCTYPE and ENTITY declarations are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" {
				return fmt.Errorf("processing instruction %q is not allowed", t.Target)
			}
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return errors.New("text outside the <svg> element")
			}
		}
	}
	if !seenRoot {
		return errors.New("root element must be <svg>")
	}
	return nil
}
