package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ScalarType is the flat settings profile's JSON type.
type ScalarType string

const (
	StringType     ScalarType = "string"
	BooleanType    ScalarType = "boolean"
	IntegerType    ScalarType = "integer"
	NumberType     ScalarType = "number"
	MaxSafeInteger            = 9007199254740991
)

// Scalar preserves false, zero and empty text. Its zero value is invalid, not null.
type Scalar struct {
	value        any
	integerExact bool
}

func String(v string) (Scalar, error) {
	if !utf8.ValidString(v) {
		return Scalar{}, errors.New("admin: invalid UTF-8 scalar")
	}
	return Scalar{value: v}, nil
}
func Boolean(v bool) Scalar { return Scalar{value: v} }
func Integer(v int64) (Scalar, error) {
	if v < -MaxSafeInteger || v > MaxSafeInteger {
		return Scalar{}, errors.New("admin: integer outside browser safe range")
	}
	return Scalar{value: float64(v), integerExact: true}, nil
}
func Number(v float64) (Scalar, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Scalar{}, errors.New("admin: nonfinite scalar")
	}
	return Scalar{value: v, integerExact: math.Trunc(v) == v && math.Abs(v) <= MaxSafeInteger}, nil
}

// Value returns the underlying string, boolean or float64 to a trusted host.
func (s Scalar) Value() any { return s.value }
func (s Scalar) valid(t ScalarType) bool {
	switch t {
	case StringType:
		_, ok := s.value.(string)
		return ok
	case BooleanType:
		_, ok := s.value.(bool)
		return ok
	case IntegerType:
		n, ok := s.value.(float64)
		return ok && s.integerExact && math.Trunc(n) == n && math.Abs(n) <= MaxSafeInteger
	case NumberType:
		_, ok := s.value.(float64)
		return ok
	}
	return false
}
func (s Scalar) MarshalJSON() ([]byte, error) {
	if s.value == nil {
		return nil, errors.New("admin: invalid scalar")
	}
	return json.Marshal(s.value)
}
func (s *Scalar) UnmarshalJSON(b []byte) error {
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return errors.New("admin: invalid scalar JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("admin: trailing scalar JSON")
	}
	var value Scalar
	var err error
	switch v := v.(type) {
	case string:
		value, err = String(v)
	case bool:
		value = Boolean(v)
	case json.Number:
		var n float64
		n, err = v.Float64()
		if err == nil {
			value, err = Number(n)
			value.integerExact = value.integerExact && integerJSONNumber(v.String())
		}
	default:
		err = errors.New("admin: expected a non-null scalar")
	}
	if err != nil {
		return errors.New("admin: invalid scalar JSON")
	}
	*s = value
	return nil
}

// Check integrality from decimal spelling before float64 rounding. This avoids
// accepting 1.0000000000000001 as an integer, without exponent-sized allocations.
func integerJSONNumber(raw string) bool {
	raw = strings.TrimPrefix(raw, "-")
	mantissa, exponent := raw, "0"
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		mantissa, exponent = raw[:i], raw[i+1:]
	}
	fractional := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fractional = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	if strings.Trim(mantissa, "0") == "" {
		return true
	}
	exp, err := strconv.Atoi(exponent)
	if err != nil {
		return false
	}
	if exp >= fractional {
		return true
	}
	if exp < -len(mantissa) {
		return false
	}
	need := fractional - exp
	return need <= len(mantissa) && strings.Trim(mantissa[len(mantissa)-need:], "0") == ""
}
