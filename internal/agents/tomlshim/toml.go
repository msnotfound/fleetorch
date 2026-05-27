package toml

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type MetaData struct{}

func DecodeFile(path string, v any) (MetaData, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return MetaData{}, err
	}
	return Decode(string(contents), v)
}

func Decode(data string, v any) (MetaData, error) {
	values, err := parse(data)
	if err != nil {
		return MetaData{}, err
	}
	if err := assign(values, v); err != nil {
		return MetaData{}, err
	}
	return MetaData{}, nil
}

func parse(data string) (map[string]any, error) {
	values := make(map[string]any)
	for lineNo, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(stripComment(line))
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d: expected key = value", lineNo+1)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo+1)
		}
		value, err := parseValue(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		values[key] = value
	}
	return values, nil
}

func stripComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return line[:i]
		}
	}
	return line
}

func parseValue(raw string) (any, error) {
	switch {
	case strings.HasPrefix(raw, "\""):
		return strconv.Unquote(raw)
	case strings.HasPrefix(raw, "["):
		return parseArray(raw)
	case raw == "true" || raw == "false":
		return strconv.ParseBool(raw)
	case strings.Contains(raw, "."):
		return strconv.ParseFloat(raw, 64)
	default:
		i, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("unsupported value %q", raw)
		}
		return i, nil
	}
}

func parseArray(raw string) ([]string, error) {
	if !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("unterminated array")
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
	if body == "" {
		return nil, nil
	}

	var out []string
	for _, part := range splitArray(body) {
		value, err := strconv.Unquote(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func splitArray(body string) []string {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for i, r := range body {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == ',' && !inString:
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	return append(parts, body[start:])
}

func assign(values map[string]any, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("decode target must be a non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("decode target must point to a struct")
	}

	rt := elem.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		key := strings.Split(field.Tag.Get("toml"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		value, ok := values[key]
		if !ok {
			continue
		}
		if err := setValue(elem.Field(i), value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func setValue(field reflect.Value, value any) error {
	if !field.CanSet() {
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string")
		}
		field.SetString(s)
	case reflect.Bool:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool")
		}
		field.SetBool(b)
	case reflect.Int:
		switch n := value.(type) {
		case int:
			field.SetInt(int64(n))
		case float64:
			field.SetInt(int64(n))
		default:
			return fmt.Errorf("expected int")
		}
	case reflect.Float64:
		switch n := value.(type) {
		case int:
			field.SetFloat(float64(n))
		case float64:
			field.SetFloat(n)
		default:
			return fmt.Errorf("expected float")
		}
	case reflect.Slice:
		items, ok := value.([]string)
		if !ok || field.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("expected string array")
		}
		field.Set(reflect.ValueOf(items))
	default:
		return fmt.Errorf("unsupported field type %s", field.Kind())
	}
	return nil
}
