package repository

import (
	"fmt"
	"strconv"
	"strings"
)

func FormatVector(vector []float32) string {
	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func ParseVector(raw string) ([]float32, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidVector)
	}
	s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	if strings.TrimSpace(s) == "" {
		return []float32{}, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, part := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidVector, err)
		}
		out = append(out, float32(f))
	}
	return out, nil
}
