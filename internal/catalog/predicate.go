package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// scalarValue reads a named scalar field off a Sample (the fixed builtin fields — see
// api.md#fields, the "scalar" row).
func scalarValue(s Sample, field string) (any, bool) {
	switch field {
	case "id":
		return float64(s.ID), true
	case "width":
		return float64(s.Width), true
	case "height":
		return float64(s.Height), true
	case "filesize":
		return float64(s.Filesize), true
	case "format":
		return s.Format, true
	case "filename":
		return s.Filename, true
	case "path":
		return s.Path, true
	case "media_type":
		return s.MediaType, true
	case "parent_id":
		return float64(s.ParentID), true
	case "group_id":
		return float64(s.GroupID), true
	case "t":
		return s.T, true
	case "slice":
		return s.Slice, true
	case "duration":
		return s.Duration, true
	case "fps":
		return s.FPS, true
	default:
		return nil, false
	}
}

// compareValue evaluates one scalar-kind operator against an already-decoded field value.
// Numbers compare as float64, everything else as string — the field's declared type picks
// which comparisons make sense, but nothing here needs to branch on it.
func compareValue(actual any, op string, raw json.RawMessage) (bool, error) {
	switch op {
	case "in", "not_in":
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil {
			return false, fmt.Errorf("%s value must be an array: %w", op, err)
		}
		found := false
		for _, item := range list {
			eq, err := compareValue(actual, "eq", item)
			if err != nil {
				return false, err
			}
			if eq {
				found = true
				break
			}
		}
		if op == "not_in" {
			return !found, nil
		}
		return found, nil
	case "contains":
		s, ok := actual.(string)
		if !ok {
			return false, fmt.Errorf("contains only applies to string fields")
		}
		var want string
		if err := json.Unmarshal(raw, &want); err != nil {
			return false, fmt.Errorf("contains value must be a string: %w", err)
		}
		return strings.Contains(s, want), nil
	}

	// eq/neq/lt/lte/gt/gte
	if n, ok := actual.(float64); ok {
		var want float64
		if err := json.Unmarshal(raw, &want); err != nil {
			return false, fmt.Errorf("expected a number for %v: %w", actual, err)
		}
		switch op {
		case "eq":
			return n == want, nil
		case "neq":
			return n != want, nil
		case "lt":
			return n < want, nil
		case "lte":
			return n <= want, nil
		case "gt":
			return n > want, nil
		case "gte":
			return n >= want, nil
		}
	}
	if s, ok := actual.(string); ok {
		var want string
		if err := json.Unmarshal(raw, &want); err != nil {
			return false, fmt.Errorf("expected a string for %v: %w", actual, err)
		}
		switch op {
		case "eq":
			return s == want, nil
		case "neq":
			return s != want, nil
		case "lt":
			return s < want, nil
		case "lte":
			return s <= want, nil
		case "gt":
			return s > want, nil
		case "gte":
			return s >= want, nil
		}
	}
	return false, fmt.Errorf("unsupported op %q for value %v", op, actual)
}

func evalTagsPredicate(tags []string, op string, raw json.RawMessage) (bool, error) {
	var want []string
	if err := json.Unmarshal(raw, &want); err != nil {
		return false, fmt.Errorf("tags predicate value must be a string array: %w", err)
	}
	has := make(map[string]bool, len(tags))
	for _, t := range tags {
		has[t] = true
	}
	switch op {
	case "all":
		for _, t := range want {
			if !has[t] {
				return false, nil
			}
		}
		return true, nil
	case "any":
		for _, t := range want {
			if has[t] {
				return true, nil
			}
		}
		return len(want) == 0, nil
	case "none":
		for _, t := range want {
			if has[t] {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("unsupported tags op %q", op)
	}
}

// evalElemMatch is true if at least one label object in list satisfies every nested predicate
// jointly (api.md#filter: elem_match).
func evalElemMatch(list []LabelValue, nested []Predicate) (bool, error) {
	for _, obj := range list {
		all := true
		for _, p := range nested {
			val, ok := obj[p.Field]
			if !ok {
				all = false
				break
			}
			ok2, err := compareValue(val, p.Op, p.Value)
			if err != nil {
				return false, err
			}
			if !ok2 {
				all = false
				break
			}
		}
		if all {
			return true, nil
		}
	}
	return false, nil
}
