package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var pgPlaceholder = regexp.MustCompile(`\$\d+`)

func interpolate(query string, args ...interface{}) string {
	if len(args) == 0 {
		return query
	}

	return pgPlaceholder.ReplaceAllStringFunc(query, func(ph string) string {
		// ambil angka setelah $
		n, err := strconv.Atoi(ph[1:])
		if err != nil || n <= 0 || n > len(args) {
			return ph // biarkan placeholder tetap apa adanya
		}

		return formatValue(args[n-1])
	})
}

func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case string:
		return escapeString(val)
	case []byte:
		return escapeString(string(val))

	// ⭐ Slice support
	case []string:
		return joinQuoted(val)
	case []int:
		return joinNumbers(val)
	case []int64:
		return joinNumbers(val)
	case []uint:
		return joinNumbers(val)
	case []uint64:
		return joinNumbers(val)
	case []any:
		return joinAny(val)

	default:
		return fmt.Sprintf("%v", val)
	}
}

func escapeString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func joinQuoted(strs []string) string {
	results := make([]string, 0, len(strs))
	for _, str := range strs {
		results = append(results, escapeString(str))
	}
	return "(" + strings.Join(results, ",") + ")"
}

func joinNumbers[T ~int | ~int64 | ~uint | ~uint64](nums []T) string {
	results := make([]string, 0, len(nums))
	for _, num := range nums {
		results = append(results, fmt.Sprintf("%v", num))
	}
	return "(" + strings.Join(results, ",") + ")"

}

func joinAny(anys []any) string {
	results := make([]string, 0, len(anys))
	for _, v := range anys {
		results = append(results, formatValue(v))
	}
	return "(" + strings.Join(results, ",") + ")"
}
