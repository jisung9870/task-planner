package domain

import (
	"fmt"
	"strings"
	"time"
)

// Duration serializes as a compact human string ("2h", "30m", "1h30m") rather
// than nanoseconds, because the frontmatter is hand-edited.
type Duration time.Duration

func ParseDuration(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// Allow "90" to mean 90 minutes - the most common thing to type.
	if !strings.ContainsAny(s, "hmsd") {
		d, err := time.ParseDuration(s + "m")
		if err != nil {
			return 0, fmt.Errorf("소요시간 형식 오류: %q (예: 30m, 2h, 1h30m)", s)
		}
		return Duration(d), nil
	}
	if i := strings.IndexByte(s, 'd'); i >= 0 {
		var days float64
		if _, err := fmt.Sscanf(s[:i], "%g", &days); err != nil {
			return 0, fmt.Errorf("소요시간 형식 오류: %q", s)
		}
		rest := Duration(0)
		if s[i+1:] != "" {
			var err error
			if rest, err = ParseDuration(s[i+1:]); err != nil {
				return 0, err
			}
		}
		return Duration(time.Duration(days*24)*time.Hour) + rest, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("소요시간 형식 오류: %q (예: 30m, 2h, 1h30m)", s)
	}
	return Duration(d), nil
}

func (d Duration) IsZero() bool       { return d == 0 }
func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d Duration) String() string {
	if d == 0 {
		return ""
	}
	td := time.Duration(d).Round(time.Minute)
	h := int(td / time.Hour)
	m := int((td % time.Hour) / time.Minute)
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func (d Duration) MarshalYAML() (any, error) {
	if d == 0 {
		return nil, nil
	}
	return d.String(), nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case nil:
		*d = 0
	case string:
		parsed, err := ParseDuration(v)
		if err != nil {
			return err
		}
		*d = parsed
	case int:
		*d = Duration(time.Duration(v) * time.Minute)
	case uint64:
		*d = Duration(time.Duration(v) * time.Minute)
	case float64:
		*d = Duration(time.Duration(v) * time.Minute)
	default:
		return fmt.Errorf("소요시간으로 해석할 수 없음: %v", raw)
	}
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = 0
		return nil
	}
	parsed, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
