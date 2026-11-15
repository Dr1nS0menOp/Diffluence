// SPDX-License-Identifier: Apache-2.0

// Package fields defines the kinds of question a template asks, what each accepts, and
// what each puts in a page (spec 074, TPL-300 to TPL-325). The form in the browser
// mirrors it in web/src/template-fields.ts, and both run testdata/cases.json, so the
// two never disagree on a case that table holds. The server decides; the browser helps.
//
// Everything here is pure: what needs the database (may the creator see this page or
// this person, does the space allow this label) is checked by the templates module.
package fields

import (
	"strings"
	"time"

	"github.com/diffluence/diffluence/internal/conditions"
)

// Kind is what a field asks for.
type Kind string

// The kinds of field. Line, text and choice are those of spec 011; the others are new.
const (
	KindLine    Kind = "line"
	KindText    Kind = "text"
	KindChoice  Kind = "choice"
	KindNumber  Kind = "number"
	KindDate    Kind = "date"
	KindYesNo   Kind = "yesno"
	KindPage    Kind = "page"
	KindPerson  Kind = "person"
	KindPeople  Kind = "people"
	KindChoices Kind = "choices"
	KindLink    Kind = "link"
	KindLabel   Kind = "label"
)

// Kinds lists every kind, in the order the template editor offers them.
var Kinds = []Kind{KindLine, KindText, KindNumber, KindDate, KindYesNo, KindChoice, KindChoices, KindPerson, KindPeople,
	KindPage, KindLink, KindLabel}

// The named formats of a line (TPL-323). There are no regular expressions (TPL-1408).
const (
	FormatEmail        = "email"
	FormatInteger      = "integer"
	FormatAlphanumeric = "alphanumeric"
)

// How some kinds show in the page (TPL-212).
const (
	ShowWords  = "words"
	ShowNone   = "none"
	ShowCommas = "commas"
	ShowList   = "list"
)

// The bounds of a field (TPL-300 to TPL-311, TPL-231, TPL-317).
const (
	MaxAnswerRunes      = 2000
	MaxHelpRunes        = 300
	MaxPlaceholderRunes = 100
	MaxOptions          = 50
	MaxOptionRunes      = 200
	MaxDecimals         = 4
	MaxPeople           = 20
	maxNumberDigits     = 15
	maxLabelRunes       = 64
	// maxOffsetDays keeps a relative date within ten years of the day the set is made.
	maxOffsetDays = 3650
)

// DateRef is a day: a fixed date, or the day the set is made moved by a number of days or
// weeks (TPL-304). It is the one relative value people need, without a formula (TPL-203).
type DateRef struct {
	Fixed  string `json:"fixed,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

// The units of a relative date.
const (
	UnitDay  = "day"
	UnitWeek = "week"
)

// Default is what a field shows filled in (TPL-316): a value of its kind, a date, or, for a
// person, the person filling in the form. Exactly one part is set.
type Default struct {
	Value   string   `json:"value,omitempty"`
	Date    *DateRef `json:"date,omitempty"`
	Creator bool     `json:"creator,omitempty"`
}

// Spec is everything a field asks beyond its name and place in the text, which the text
// holds (TPL-1431). Only the settings its kind has may be set.
type Spec struct {
	Name        string   `json:"name"`
	Kind        Kind     `json:"kind"`
	Required    bool     `json:"required"`
	Help        string   `json:"help,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	MinLength   *int     `json:"min_length,omitempty"`
	MaxLength   *int     `json:"max_length,omitempty"`
	Format      string   `json:"format,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Decimals    int      `json:"decimals,omitempty"`
	Earliest    *DateRef `json:"earliest,omitempty"`
	Latest      *DateRef `json:"latest,omitempty"`
	MinChoices  *int     `json:"min_choices,omitempty"`
	MaxChoices  *int     `json:"max_choices,omitempty"`
	// SpaceID limits a page field to the pages of one space (TPL-306).
	SpaceID string   `json:"space_id,omitempty"`
	Show    string   `json:"show,omitempty"`
	Default *Default `json:"default,omitempty"`
	// DefaultGone says the suggested person is no longer active, so the form shows the field
	// empty with a note (TPL-316). The server sets it when it hands out a form; it is never stored.
	DefaultGone bool `json:"default_gone,omitempty"`
	// Condition shows the question only when it holds, testing questions asked before it
	// (spec 074, TPL-421). The templates module checks it, because only it knows the form.
	Condition *conditions.Condition `json:"condition,omitempty"`
}

// Problem is why an answer or a field is refused, as a code the browser translates and
// the values its words need.
type Problem struct {
	Code   string            `json:"problem"`
	Params map[string]string `json:"params,omitempty"`
}

func problem(code string, params ...string) *Problem {
	p := &Problem{Code: code}
	if len(params) > 0 {
		p.Params = map[string]string{}
		for i := 0; i+1 < len(params); i += 2 {
			p.Params[params[i]] = params[i+1]
		}
	}
	return p
}

// The words of each problem, in English, for an answer of the API. The form has its own,
// in the translation files, under templates.problem.
var messages = map[string]string{
	"required":           "Fill this in.",
	"one_line":           "Keep this to one line.",
	"too_short":          "Enter at least {min} characters.",
	"too_long":           "Enter at most {max} characters.",
	"not_an_option":      "Choose one of the options.",
	"not_a_number":       "Enter a number, such as 12 or 3.5.",
	"too_many_decimals":  "Enter a number with at most {decimals} digits after the decimal sign.",
	"below_min":          "Enter a number of at least {min}.",
	"above_max":          "Enter a number of at most {max}.",
	"not_a_date":         "Enter a date that exists, such as 2027-03-01.",
	"too_early":          "Enter a date on or after {date}.",
	"too_late":           "Enter a date on or before {date}.",
	"not_yes_no":         "Answer yes or no.",
	"not_an_id":          "Choose from the list.",
	"too_many_people":    "Choose at most {max} people.",
	"repeated":           "Choose each one once.",
	"too_few_choices":    "Choose at least {min}.",
	"too_many_choices":   "Choose at most {max}.",
	"not_a_link":         "Enter a web address that starts with https:// or http://.",
	"not_an_email":       "Enter an email address, such as name@example.org.",
	"not_a_whole_number": "Enter a whole number, such as 12.",
	"not_letters_digits": "Use only letters and digits.",
	"not_a_label":        "A label has letters, digits, dashes and underscores, and at most 64 characters.",
	"page_not_found":     "This page does not exist, or you cannot see it. Choose another.",
	"page_other_space":   "Choose a page of the space this question names.",
	"person_not_found":   "This person does not exist, or you cannot see them. Choose another.",
	"person_inactive":    "This person is no longer active. Choose another.",
	"label_not_allowed":  "This space only takes labels from its list. Choose one of them.",
	"not_asked":          "This template does not ask this question.",

	"unknown_kind":         "This kind of field does not exist.",
	"yesno_required":       "A yes/no field cannot be required, because no is an answer too.",
	"help_too_long":        "Help text can have at most 300 characters.",
	"placeholder_too_long": "An example in the field can have at most 100 characters.",
	"needs_options":        "Give at least one option.",
	"too_many_options":     "A field can have at most 50 options.",
	"bad_option":           "Each option is one line of at most 200 characters, and appears once.",
	"setting_not_for_kind": "This kind of field does not have this setting.",
	"bad_format":           "Choose email address, whole number, or letters and digits.",
	"bad_length":           "The lengths must be between 0 and 2000, the shortest no longer than the longest.",
	"bad_decimals":         "A number can have 0 to 4 decimals.",
	"bad_range":            "The lowest number cannot be above the highest.",
	"bad_date_bound":       "A date limit is a date, or a number of days or weeks from the day the pages are made, within ten years; the earliest cannot be after the latest.",
	"bad_choices":          "The number to choose must fit the options, the least no more than the most.",
	"bad_space":            "Choose a space for the page field.",
	"bad_show":             "This kind of field cannot be shown that way.",
	"bad_default":          "The suggested answer does not fit this field. Change it or remove it.",
	"default_inactive":     "The suggested person is no longer active. Choose another, or remove the suggestion.",
	"reserved_name":        "This name is kept for a value the template knows. Choose another name.",
	"names_differ_in_case": "Two questions differ only in capitals. Give them names that differ.",
}

// problemCodes lists every code, for the test that each has words.
var problemCodes = func() []string {
	out := make([]string, 0, len(messages))
	for code := range messages {
		out = append(out, code)
	}
	return out
}()

// Message gives the problem in words, with dates written out in full.
func (p Problem) Message() string {
	msg := messages[p.Code]
	for key, value := range p.Params {
		if key == "date" {
			if day, err := time.Parse(time.DateOnly, value); err == nil {
				value = day.Format("2 January 2006")
			}
		}
		msg = strings.ReplaceAll(msg, "{"+key+"}", value)
	}
	return msg
}
