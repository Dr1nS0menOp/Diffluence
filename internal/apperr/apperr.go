// SPDX-License-Identifier: Apache-2.0

// Package apperr holds the errors that modules return for expected situations.
// The HTTP layer turns them into status codes; anything else is a 500.
package apperr

import "errors"

// Kind classifies an error.
type Kind int

// Kinds of expected errors.
const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindForbidden
	KindConflict
	KindTooLarge
	KindBusy
	KindNoRoom
)

// Denied describes a refused action, so it can be audited after the
// transaction it happened in has rolled back.
type Denied struct {
	Action     string
	ObjectType string
	ObjectID   string
	SpaceID    string
}

// FieldProblem is one field of a form that was refused, so the form can show the words
// next to that field and keep every other answer (spec 074, TPL-324). Problem is a code
// the browser translates; Message is the same in English, for any other client.
type FieldProblem struct {
	Field   string            `json:"field"`
	Problem string            `json:"problem"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"message"`
}

// Error is an expected failure with a message that is safe to show to users.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Denied  *Denied
	// Fields names each field of a form that was refused, when the failure is about fields.
	Fields []FieldProblem
}

// InvalidFields means some fields of a form were refused, each named with why.
func InvalidFields(code string, problems []FieldProblem) *Error {
	message := "Some answers need a change."
	for _, p := range problems {
		message += " " + p.Field + ": " + p.Message
	}
	return &Error{Kind: KindInvalid, Code: code, Message: message, Fields: problems}
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Invalid means the input was wrong.
func Invalid(message string) *Error {
	return &Error{Kind: KindInvalid, Code: "invalid_input", Message: message}
}

// NotFound means the object does not exist or may not be seen.
func NotFound() *Error {
	return &Error{Kind: KindNotFound, Code: "not_found", Message: "Not found."}
}

// Forbidden means the user can see the object but may not do this.
func Forbidden() *Error {
	return &Error{Kind: KindForbidden, Code: "forbidden", Message: "You do not have permission to do this."}
}

// Conflict means the action does not fit the current state.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// TooLarge means the input exceeded a limit.
func TooLarge(message string) *Error {
	return &Error{Kind: KindTooLarge, Code: "too_large", Message: message}
}

// Busy means there is too much work right now, and the same request can work in a moment.
func Busy(message string) *Error {
	return &Error{Kind: KindBusy, Code: "busy", Message: message}
}

// NoRoom means the server has too little disk left to store this, while smaller work,
// such as saving text, still goes on (spec 051, D4).
func NoRoom(message string) *Error {
	return &Error{Kind: KindNoRoom, Code: "no_room", Message: message}
}

// As extracts an *Error from err.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// IsNotFound reports whether err is a not-found error.
func IsNotFound(err error) bool {
	e, ok := As(err)
	return ok && e.Kind == KindNotFound
}
