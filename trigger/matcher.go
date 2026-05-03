package trigger

import (
	"path/filepath"
	"strings"
)

type Matcher interface {
	Match(Event) (bool, error)
	Describe() string
}

type MatchAll struct{}

func (MatchAll) Match(Event) (bool, error) { return true, nil }
func (MatchAll) Describe() string          { return "all" }

type EventType string

func (m EventType) Match(event Event) (bool, error) { return event.Type == string(m), nil }
func (m EventType) Describe() string                { return "event.type == " + string(m) }

type SourceIs SourceID

func (m SourceIs) Match(event Event) (bool, error) { return event.SourceID == SourceID(m), nil }
func (m SourceIs) Describe() string                { return "source == " + string(m) }

type SubjectGlob string

func (m SubjectGlob) Match(event Event) (bool, error) {
	if strings.TrimSpace(string(m)) == "" {
		return true, nil
	}
	return filepath.Match(string(m), event.Subject)
}
func (m SubjectGlob) Describe() string { return "subject matches " + string(m) }

type All []Matcher

func (m All) Match(event Event) (bool, error) {
	for _, matcher := range m {
		if matcher == nil {
			continue
		}
		ok, err := matcher.Match(event)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}
func (m All) Describe() string {
	parts := make([]string, 0, len(m))
	for _, matcher := range m {
		if matcher != nil {
			parts = append(parts, matcher.Describe())
		}
	}
	return strings.Join(parts, " && ")
}

type Any []Matcher

func (m Any) Match(event Event) (bool, error) {
	for _, matcher := range m {
		if matcher == nil {
			continue
		}
		ok, err := matcher.Match(event)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
func (m Any) Describe() string {
	parts := make([]string, 0, len(m))
	for _, matcher := range m {
		if matcher != nil {
			parts = append(parts, matcher.Describe())
		}
	}
	return strings.Join(parts, " || ")
}
