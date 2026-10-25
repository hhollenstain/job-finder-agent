package source

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FlexString accepts a JSON string or number.
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(strings.TrimSpace(string(b)))
	return nil
}

func (f FlexString) String() string { return string(f) }

// FlexInt accepts a JSON number or numeric string. Non-numeric text becomes 0.
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || len(b) == 0 {
		return nil
	}
	var n float64
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		parsed, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		*f = FlexInt(parsed)
		return nil
	}
	if err := json.Unmarshal(b, &n); err != nil {
		return nil
	}
	*f = FlexInt(n)
	return nil
}
