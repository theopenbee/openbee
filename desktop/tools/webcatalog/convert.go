package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/i18n"
)

var pluralCategories = []string{"one", "other"}

var (
	identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	flatName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

func FlatKey(path []string) string {
	var b strings.Builder
	first := true
	for _, segment := range path {
		words := strings.FieldsFunc(segment, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		for _, word := range words {
			if first {
				b.WriteString(word)
				first = false
				continue
			}
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			b.WriteString(string(runes))
		}
	}
	return b.String()
}

func ToICU(s string) string {
	out, _ := convertText(s, false)
	return out
}

func convertText(s string, plural bool) (string, bool) {
	var out, literal strings.Builder
	malformed := false
	flush := func() {
		out.WriteString(escapeLiteral(literal.String(), plural))
		literal.Reset()
	}
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "{{") {
			if end := strings.Index(s[i:], "}}"); end >= 0 {
				name := strings.TrimSpace(s[i+2 : i+end])
				if identifier.MatchString(name) {
					flush()
					if plural && name == "count" {
						out.WriteByte('#')
					} else {
						out.WriteString("{" + name + "}")
					}
					i += end + 2
					continue
				}
			}
			malformed = true
		}
		literal.WriteByte(s[i])
		i++
	}
	flush()
	return out.String(), malformed
}

func escapeLiteral(s string, plural bool) string {
	special := func(c byte) bool {
		return c == '{' || c == '}' || c == '\'' || plural && c == '#'
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if !special(s[i]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		quote := false
		for j < len(s) && special(s[j]) {
			if s[j] != '\'' {
				quote = true
			}
			j++
		}
		span := s[i:j]
		switch {
		case quote:
			b.WriteString("'" + strings.ReplaceAll(span, "'", "''") + "'")
		case len(span) == 1 && j < len(s):
			b.WriteString(span)
		default:
			b.WriteString(strings.ReplaceAll(span, "'", "''"))
		}
		i = j
	}
	return b.String()
}

type pluralGroup struct {
	forms  map[string]string
	origin string
}

type converter struct {
	messages map[string]string
	origins  map[string]string
	plurals  map[string]*pluralGroup
	order    []string
}

func Convert(web map[string]any, overlay map[string]string) (map[string]string, map[string][]string, error) {
	c := &converter{
		messages: map[string]string{},
		origins:  map[string]string{},
		plurals:  map[string]*pluralGroup{},
	}
	if err := c.walk(nil, web); err != nil {
		return nil, nil, err
	}
	for _, flat := range c.order {
		group := c.plurals[flat]
		message, err := pluralMessage(group)
		if err != nil {
			return nil, nil, err
		}
		if err := c.add(flat, group.origin, message); err != nil {
			return nil, nil, err
		}
	}
	names := make([]string, 0, len(overlay))
	for name := range overlay {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !flatName.MatchString(name) {
			return nil, nil, fmt.Errorf("overlay key %q is not a flat key of letters, digits and underscores", name)
		}
		if err := c.add(name, "overlay key "+name, overlay[name]); err != nil {
			return nil, nil, err
		}
	}
	placeholders := map[string][]string{}
	for key, message := range c.messages {
		parsed, err := i18n.ParseMessage(message)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", c.origins[key], err)
		}
		var names []string
		for _, arg := range parsed.Args() {
			names = append(names, arg.Name)
		}
		if len(names) > 0 {
			placeholders[key] = names
		}
	}
	return c.messages, placeholders, nil
}

func (c *converter) walk(path []string, node map[string]any) error {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		next := append(append([]string(nil), path...), key)
		switch value := node[key].(type) {
		case map[string]any:
			if err := c.walk(next, value); err != nil {
				return err
			}
		case string:
			if err := c.leaf(next, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unsupported value %T", strings.Join(next, "."), value)
		}
	}
	return nil
}

func (c *converter) leaf(path []string, value string) error {
	dotted := strings.Join(path, ".")
	last := path[len(path)-1]
	for _, category := range pluralCategories {
		suffix := "_" + category
		if !strings.HasSuffix(last, suffix) || last == suffix {
			continue
		}
		base := append(append([]string(nil), path[:len(path)-1]...), strings.TrimSuffix(last, suffix))
		flat := FlatKey(base)
		group, ok := c.plurals[flat]
		if !ok {
			group = &pluralGroup{forms: map[string]string{}, origin: dotted}
			c.plurals[flat] = group
			c.order = append(c.order, flat)
		}
		group.forms[category] = value
		return nil
	}
	icu, malformed := convertText(value, false)
	if malformed {
		return fmt.Errorf("%s: unsupported interpolation in %q", dotted, value)
	}
	return c.add(FlatKey(path), dotted, icu)
}

func (c *converter) add(flat, origin, message string) error {
	if flat == "" {
		return fmt.Errorf("%s: empty flat key", origin)
	}
	if previous, ok := c.origins[flat]; ok {
		return fmt.Errorf("keys %q and %q both map to %q", previous, origin, flat)
	}
	c.origins[flat] = origin
	c.messages[flat] = message
	return nil
}

func pluralMessage(group *pluralGroup) (string, error) {
	if _, ok := group.forms["other"]; !ok {
		return "", fmt.Errorf("%s: plural forms need an _other form", group.origin)
	}
	var cases []string
	for _, category := range pluralCategories {
		form, ok := group.forms[category]
		if !ok {
			continue
		}
		text, malformed := convertText(form, true)
		if malformed {
			return "", fmt.Errorf("%s: unsupported interpolation in %q", group.origin, form)
		}
		cases = append(cases, category+" {"+text+"}")
	}
	return "{count, plural, " + strings.Join(cases, " ") + "}", nil
}

func Declare(source, other map[string][]string) {
	for key, names := range other {
		for _, name := range names {
			if !slices.Contains(source[key], name) {
				source[key] = append(source[key], name)
			}
		}
	}
}

func ARB(locale string, messages map[string]string, placeholders map[string][]string) []byte {
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteString("{\n  \"@@locale\": " + quote(locale))
	for _, key := range keys {
		b.WriteString(",\n  " + quote(key) + ": " + quote(messages[key]))
		names := placeholders[key]
		if len(names) == 0 {
			continue
		}
		entries := make([]string, len(names))
		for i, name := range names {
			kind := "String"
			if name == "count" {
				kind = "int"
			}
			entries[i] = quote(name) + ": { \"type\": " + quote(kind) + " }"
		}
		b.WriteString(",\n  " + quote("@"+key) + ": { \"placeholders\": { " + strings.Join(entries, ", ") + " } }")
	}
	b.WriteString("\n}\n")
	return b.Bytes()
}

func quote(s string) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

func ErrorsSource(web map[string]any, placeholders map[string][]string) ([]byte, error) {
	section, ok := web["errors"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the web catalog has no errors section")
	}
	codes := make([]string, 0, len(section))
	for code, value := range section {
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("errors.%s is not a string", code)
		}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/webcatalog; DO NOT EDIT.\n\npackage l10n\n\nimport \"github.com/theopenbee/openbee/desktop/locales\"\n\n")
	b.WriteString("func errorText(code string, params map[string]any) (string, bool) {\n\tswitch code {\n")
	for _, code := range codes {
		flat := FlatKey([]string{"errors", code})
		args := make([]string, 0, len(placeholders[flat]))
		for _, name := range placeholders[flat] {
			if name == "count" {
				return nil, fmt.Errorf("errors.%s: count placeholders are not supported", code)
			}
			args = append(args, fmt.Sprintf("param(params, %q)", name))
		}
		fmt.Fprintf(&b, "\tcase %q:\n\t\treturn locales.%s(%s), true\n", code, exported(flat), strings.Join(args, ", "))
	}
	b.WriteString("\t}\n\treturn \"\", false\n}\n")
	return format.Source(b.Bytes())
}

func exported(flat string) string {
	runes := []rune(flat)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
