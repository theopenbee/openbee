package plugin

import (
	"fmt"
	"math"
	"slices"
)

func ResolveConfig(m Manifest, section map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(section))
	for k, v := range section {
		if slices.Contains(reservedConfigKeys, k) {
			continue
		}
		out[k] = v
	}
	for _, f := range m.Config {
		v, present := out[f.Key]
		if !present || v == nil {
			delete(out, f.Key)
			if f.Default == nil {
				if f.Required {
					return nil, fmt.Errorf("config %q is required", f.Key)
				}
				continue
			}
			v = f.Default
		}
		coerced, err := coerce(f, v)
		if err != nil {
			return nil, err
		}
		if f.Required && isEmpty(coerced) {
			return nil, fmt.Errorf("config %q is required", f.Key)
		}
		out[f.Key] = coerced
	}
	return out, nil
}

func coerce(f ConfigField, v any) (any, error) {
	switch f.Type {
	case FieldString, FieldSecret:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config %q must be a string", f.Key)
		}
		return s, nil
	case FieldBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("config %q must be a boolean", f.Key)
		}
		return b, nil
	case FieldInt:
		switch n := v.(type) {
		case int:
			return n, nil
		case int64:
			return int(n), nil
		case uint64:
			return int(n), nil
		case float64:
			if n == math.Trunc(n) {
				return int(n), nil
			}
		}
		return nil, fmt.Errorf("config %q must be an integer", f.Key)
	case FieldList:
		switch items := v.(type) {
		case []string:
			return items, nil
		case []any:
			out := make([]string, 0, len(items))
			for _, it := range items {
				s, ok := it.(string)
				if !ok {
					return nil, fmt.Errorf("config %q must be a list of strings", f.Key)
				}
				out = append(out, s)
			}
			return out, nil
		}
		return nil, fmt.Errorf("config %q must be a list of strings", f.Key)
	}
	return nil, fmt.Errorf("config %q has unknown type %q", f.Key, f.Type)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case []string:
		return len(x) == 0
	}
	return false
}
