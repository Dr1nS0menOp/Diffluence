// SPDX-License-Identifier: Apache-2.0

package fields

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Answer checks one answer against its field and gives it as it lands in the page, or the
// problem with it. today is the day the set is made, as YYYY-MM-DD. An empty answer is no
// answer: only a required field refuses it. Several people or options come one per line.
func Answer(s Spec, raw string, today string) (string, *Problem) {
	if !utf8.ValidString(raw) {
		return "", problem("not_an_id")
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		if s.Required && s.Kind != KindYesNo {
			return "", problem("required")
		}
		return "", nil
	}
	switch s.Kind {
	case KindText:
		return value, lengthProblem(s, value)
	case KindChoice:
		if !slices.Contains(s.Options, value) {
			return "", problem("not_an_option")
		}
		return value, nil
	case KindNumber:
		return value, numberProblem(s, value)
	case KindDate:
		return value, dateProblem(s, value, today)
	case KindYesNo:
		if value != "yes" && value != "no" {
			return "", problem("not_yes_no")
		}
		return value, nil
	case KindPage, KindPerson:
		if !IsID(value) {
			return "", problem("not_an_id")
		}
		return value, nil
	case KindPeople:
		return listAnswer(value, func(item string) *Problem {
			if !IsID(item) {
				return problem("not_an_id")
			}
			return nil
		}, s)
	case KindChoices:
		return listAnswer(value, func(item string) *Problem {
			if !slices.Contains(s.Options, item) {
				return problem("not_an_option")
			}
			return nil
		}, s)
	case KindLink:
		return value, linkProblem(value)
	case KindLabel:
		return labelAnswer(value)
	default: // a line, and any kind an older text names
		if strings.ContainsAny(value, "\r\n") {
			return "", problem("one_line")
		}
		if p := lengthProblem(s, value); p != nil {
			return "", p
		}
		return value, formatProblem(s.Format, value)
	}
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// IsID accepts an id only in the one form the product writes, so the form in the browser
// and the server agree on what an id looks like.
func IsID(value string) bool {
	if len(value) != len("01938f2e-7b3a-7cde-8f00-0123456789ab") {
		return false
	}
	_, err := uuid.Parse(value)
	return err == nil
}

func lengthProblem(s Spec, value string) *Problem {
	n := runes(value)
	most := MaxAnswerRunes
	if s.MaxLength != nil && *s.MaxLength < most {
		most = *s.MaxLength
	}
	if n > most {
		return problem("too_long", "max", strconv.Itoa(most))
	}
	if s.MinLength != nil && n < *s.MinLength {
		return problem("too_short", "min", strconv.Itoa(*s.MinLength))
	}
	return nil
}

func formatProblem(format, value string) *Problem {
	switch format {
	case FormatEmail:
		if !isEmail(value) {
			return problem("not_an_email")
		}
	case FormatInteger:
		if !isDigits(strings.TrimPrefix(value, "-")) {
			return problem("not_a_whole_number")
		}
	case FormatAlphanumeric:
		for _, r := range value {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return problem("not_letters_digits")
			}
		}
	}
	return nil
}

// isEmail is the plain shape of an address: one @, something before it, and a domain with a
// dot that neither starts nor ends it. Whether the address exists is not for a form to know.
func isEmail(value string) bool {
	local, domain, found := strings.Cut(value, "@")
	if !found || local == "" || strings.Contains(domain, "@") || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	dot := strings.LastIndex(domain, ".")
	return dot > 0 && dot < len(domain)-1 && !strings.HasPrefix(domain, ".")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ParseNumber reads a number written with a point or a comma as the decimal sign and no
// other separator, and counts its decimals (TPL-303). ok is false for anything else.
func ParseNumber(value string) (n float64, decimals int, ok bool) {
	digits := strings.TrimPrefix(value, "-")
	whole, fraction, separated := strings.Cut(strings.ReplaceAll(digits, ",", "."), ".")
	if !isDigits(whole) || len(whole) > maxNumberDigits || (separated && !isDigits(fraction)) {
		return 0, 0, false
	}
	n, err := strconv.ParseFloat(strings.Replace(value, ",", ".", 1), 64)
	return n, len(fraction), err == nil
}

func formatNumber(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

func numberProblem(s Spec, value string) *Problem {
	n, decimals, ok := ParseNumber(value)
	if !ok {
		return problem("not_a_number")
	}
	if decimals > s.Decimals {
		return problem("too_many_decimals", "decimals", strconv.Itoa(s.Decimals))
	}
	if s.Min != nil && n < *s.Min {
		return problem("below_min", "min", formatNumber(*s.Min))
	}
	if s.Max != nil && n > *s.Max {
		return problem("above_max", "max", formatNumber(*s.Max))
	}
	return nil
}

// parseDay reads a calendar date. Dates are days, not moments, so no time zone moves them.
func parseDay(value string) (time.Time, bool) {
	if len(value) != len(time.DateOnly) {
		return time.Time{}, false
	}
	day, err := time.Parse(time.DateOnly, value)
	return day, err == nil
}

// Resolve gives the day a DateRef names, counted from today.
func (d DateRef) Resolve(today string) (string, bool) {
	if d.Fixed != "" {
		_, ok := parseDay(d.Fixed)
		return d.Fixed, ok
	}
	base, ok := parseDay(today)
	if !ok {
		return "", false
	}
	days := d.Offset
	if d.Unit == UnitWeek {
		days *= 7
	}
	return base.AddDate(0, 0, days).Format(time.DateOnly), true
}

func dateProblem(s Spec, value, today string) *Problem {
	if _, ok := parseDay(value); !ok {
		return problem("not_a_date")
	}
	// Days written as YYYY-MM-DD sort as text in the order of the calendar.
	if s.Earliest != nil {
		if earliest, ok := s.Earliest.Resolve(today); ok && value < earliest {
			return problem("too_early", "date", earliest)
		}
	}
	if s.Latest != nil {
		if latest, ok := s.Latest.Resolve(today); ok && value > latest {
			return problem("too_late", "date", latest)
		}
	}
	return nil
}

// listAnswer checks an answer of several items, one per line.
func listAnswer(value string, check func(string) *Problem, s Spec) (string, *Problem) {
	items := []string{}
	for _, line := range strings.Split(value, "\n") {
		item := strings.TrimSpace(line)
		if item == "" {
			continue
		}
		if p := check(item); p != nil {
			return "", p
		}
		if slices.Contains(items, item) {
			return "", problem("repeated")
		}
		items = append(items, item)
	}
	if s.Kind == KindPeople && len(items) > MaxPeople {
		return "", problem("too_many_people", "max", strconv.Itoa(MaxPeople))
	}
	if s.MinChoices != nil && len(items) < *s.MinChoices {
		return "", problem("too_few_choices", "min", strconv.Itoa(*s.MinChoices))
	}
	if s.MaxChoices != nil && len(items) > *s.MaxChoices {
		return "", problem("too_many_choices", "max", strconv.Itoa(*s.MaxChoices))
	}
	return strings.Join(items, "\n"), nil
}

// linkProblem allows a web address of http or https only, so a typed javascript: address
// never becomes a link (TPL-310).
func linkProblem(value string) *Problem {
	if runes(value) > MaxAnswerRunes {
		return problem("too_long", "max", strconv.Itoa(MaxAnswerRunes))
	}
	if strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return problem("not_a_link")
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http")) {
		return problem("not_a_link")
	}
	return nil
}

// labelAnswer writes a label the one allowed way, as the labels module does (ORG-043).
func labelAnswer(value string) (string, *Problem) {
	label := strings.Join(strings.Fields(strings.ToLower(value)), "-")
	if runes(label) > maxLabelRunes {
		return "", problem("not_a_label")
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", problem("not_a_label")
		}
	}
	return label, nil
}

// DefaultValue is the answer a field starts with: its value, its day counted from today, or
// the creator. It is checked as any answer is.
func (s Spec) DefaultValue(today, creatorID string) string {
	switch {
	case s.Default == nil:
		return ""
	case s.Default.Creator:
		return creatorID
	case s.Default.Date != nil:
		day, _ := s.Default.Date.Resolve(today)
		return day
	}
	return s.Default.Value
}
