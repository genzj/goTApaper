package config

import (
	"encoding/json"
	"strings"
)

type SizeStrategy int

const (
	Unknown       SizeStrategy = -1
	LargestNoLogo              = iota
	Largest
	ByWidth
)

func (a *SizeStrategy) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	default:
		*a = Unknown
	case "largest-no-logo":
		*a = LargestNoLogo
	case "largest":
		*a = Largest
	case "by-width":
		*a = ByWidth
	}

	return nil
}

func (a SizeStrategy) MarshalJSON() ([]byte, error) {
	var s string
	switch a {
	default:
		s = "unknown"
	case LargestNoLogo:
		s = "largest-no-logo"
	case Largest:
		s = "largest"
	case ByWidth:
		s = "by-width"
	}

	return json.Marshal(s)
}
