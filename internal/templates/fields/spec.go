// SPDX-License-Identifier: Apache-2.0

package fields

import (
	"slices"
	"strings"
)

// settings names what each kind may set beyond its name, required, help and default.
var settings = map[Kind][]string{
	KindLine:    {"placeholder", "length", "format"},
	KindText:    {"placeholder", "length"},
	KindChoice:  {"options"},
	KindNumber:  {"placeholder", "number"},
	KindDate:    {"dates"},
	KindYesNo:   {"show"},
	KindPage:    {"space"},
	KindPerson:  {},
	KindPeople:  {},
	KindChoices: {"options", "choices", "show"},
	KindLink:    {"placeholder"},
	KindLabel:   {"placeholder"},
}

// used lists the settings a spec sets.
func (s Spec) used() []string {
	out := []string{}
	add := func(set bool, name string) {
		if set && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	add(s.Placeholder != "", "placeholder")
	add(s.MinLength != nil || s.MaxLength != nil, "length")
	add(s.Format != "", "format")
	add(len(s.Options) > 0, "options")
	add(s.Min != nil || s.Max != nil || s.Decimals != 0, "number")
	add(s.Earliest != nil || s.Latest != nil, "dates")
	add(s.MinChoices != nil || s.MaxChoices != nil, "choices")
	add(s.SpaceID != "", "space")
	add(s.Show != "", "show")
	return out
}

// Check validates a field as its author wrote it, before the template is saved (TPL-300 to
// TPL-317). today is the day of the save, for a default counted from it. What needs the
// database, such as whether a suggested person is still active, the templates module checks.
func (s Spec) Check(today string) *Problem {
	allowed, known := settings[s.Kind]
	switch {
	case !known:
		return problem("unknown_kind")
	case s.Kind == KindYesNo && s.Required:
		return problem("yesno_required")
	case runes(s.Help) > MaxHelpRunes:
		return problem("help_too_long")
	case runes(s.Placeholder) > MaxPlaceholderRunes:
		return problem("placeholder_too_long")
	}
	for _, setting := range s.used() {
		if !slices.Contains(allowed, setting) {
			return problem("setting_not_for_kind")
		}
	}
	for _, check := range []func() *Problem{s.checkOptions, s.checkLength, s.checkFormat, s.checkNumber, s.checkDates,
		s.checkChoices, s.checkSpace, s.checkShow} {
		if p := check(); p != nil {
			return p
		}
	}
	return s.checkDefault(today)
}

func (s Spec) checkOptions() *Problem {
	if s.Kind != KindChoice && s.Kind != KindChoices {
		return nil
	}
	if len(s.Options) == 0 {
		return problem("needs_options")
	}
	if len(s.Options) > MaxOptions {
		return problem("too_many_options")
	}
	for i, option := range s.Options {
		if strings.TrimSpace(option) == "" || runes(option) > MaxOptionRunes || strings.ContainsAny(option, "\r\n") ||
			slices.Contains(s.Options[:i], option) {
			return problem("bad_option")
		}
	}
	return nil
}

func (s Spec) checkLength() *Problem {
	low, high := 0, MaxAnswerRunes
	if s.MinLength != nil {
		low = *s.MinLength
	}
	if s.MaxLength != nil {
		high = *s.MaxLength
	}
	if low < 0 || high > MaxAnswerRunes || low > high {
		return problem("bad_length")
	}
	return nil
}

func (s Spec) checkFormat() *Problem {
	if s.Format != "" && !slices.Contains([]string{FormatEmail, FormatInteger, FormatAlphanumeric}, s.Format) {
		return problem("bad_format")
	}
	return nil
}

func (s Spec) checkNumber() *Problem {
	if s.Decimals < 0 || s.Decimals > MaxDecimals {
		return problem("bad_decimals")
	}
	if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
		return problem("bad_range")
	}
	return nil
}

func (d *DateRef) valid() bool {
	if d == nil {
		return true
	}
	if d.Fixed != "" {
		_, ok := parseDay(d.Fixed)
		return ok && d.Offset == 0 && d.Unit == ""
	}
	days := d.Offset
	switch d.Unit {
	case "", UnitDay:
	case UnitWeek:
		days *= 7
	default:
		return false
	}
	return days >= -maxOffsetDays && days <= maxOffsetDays
}

func (s Spec) checkDates() *Problem {
	if !s.Earliest.valid() || !s.Latest.valid() {
		return problem("bad_date_bound")
	}
	// Only two fixed days can be compared once and for all; relative ones move together.
	if s.Earliest != nil && s.Latest != nil && s.Earliest.Fixed != "" && s.Latest.Fixed != "" && s.Earliest.Fixed > s.Latest.Fixed {
		return problem("bad_date_bound")
	}
	return nil
}

func (s Spec) checkChoices() *Problem {
	low, high := 0, len(s.Options)
	if s.MinChoices != nil {
		low = *s.MinChoices
	}
	if s.MaxChoices != nil {
		high = *s.MaxChoices
	}
	if low < 0 || high > len(s.Options) || low > high {
		return problem("bad_choices")
	}
	return nil
}

func (s Spec) checkSpace() *Problem {
	if s.SpaceID != "" && !IsID(s.SpaceID) {
		return problem("bad_space")
	}
	return nil
}

func (s Spec) checkShow() *Problem {
	switch {
	case s.Show == "":
		return nil
	case s.Kind == KindYesNo && (s.Show == ShowWords || s.Show == ShowNone):
		return nil
	case s.Kind == KindChoices && (s.Show == ShowCommas || s.Show == ShowList):
		return nil
	}
	return problem("bad_show")
}

// checkDefault refuses a default that is not an answer of its own kind, such as an option
// that was removed (TPL-316).
func (s Spec) checkDefault(today string) *Problem {
	d := s.Default
	if d == nil {
		return nil
	}
	parts := 0
	for _, set := range []bool{d.Value != "", d.Date != nil, d.Creator} {
		if set {
			parts++
		}
	}
	wrongPart := (d.Creator && s.Kind != KindPerson && s.Kind != KindPeople) ||
		(d.Date != nil) != (s.Kind == KindDate) || !d.Date.valid()
	if parts != 1 || wrongPart {
		return problem("bad_default")
	}
	if d.Creator {
		return nil
	}
	loose := s
	loose.Required = false
	if _, p := Answer(loose, s.DefaultValue(today, ""), today); p != nil {
		return problem("bad_default")
	}
	return nil
}
