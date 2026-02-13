package sqlh

import (
	"fmt"

	"github.com/gosoline-project/sqlr"
	"github.com/spf13/cast"
)

// parseKeyFromString converts a string to any type constrained by sqlr.KeyTypes.
// It handles both value and pointer types by using type switches to dispatch to
// the appropriate cast.To*E function, then converting to a pointer if needed.
func parseKeyFromString[K sqlr.KeyTypes](raw string) (K, error) {
	var zero K
	var result any
	var err error

	// First, parse the string to the base type
	switch any(zero).(type) {
	case bool, *bool:
		result, err = cast.ToBoolE(raw)
	case string, *string:
		result, err = cast.ToStringE(raw)
	case int, *int:
		result, err = cast.ToIntE(raw)
	case int64, *int64:
		result, err = cast.ToInt64E(raw)
	case uint, *uint:
		result, err = cast.ToUintE(raw)
	case uint64, *uint64:
		result, err = cast.ToUint64E(raw)
	case float32, *float32:
		result, err = cast.ToFloat32E(raw)
	case float64, *float64:
		result, err = cast.ToFloat64E(raw)
	default:
		return zero, fmt.Errorf("unsupported key type %T", zero)
	}

	if err != nil {
		return zero, err
	}

	// If K is a pointer type, convert the result to a pointer
	switch any(zero).(type) {
	case *bool:
		v := result.(bool)
		result = &v
	case *string:
		v := result.(string)
		result = &v
	case *int:
		v := result.(int)
		result = &v
	case *int64:
		v := result.(int64)
		result = &v
	case *uint:
		v := result.(uint)
		result = &v
	case *uint64:
		v := result.(uint64)
		result = &v
	case *float32:
		v := result.(float32)
		result = &v
	case *float64:
		v := result.(float64)
		result = &v
	}

	return result.(K), nil
}
