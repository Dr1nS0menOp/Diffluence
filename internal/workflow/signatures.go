// SPDX-License-Identifier: Apache-2.0

package workflow

import "regexp"

// Electronic signatures (blueprint 09.8, spec 064 wave DC4). A transition names the meaning
// of the signature it requires; the meaning is what the signer states, shown before signing
// and kept in the record (DOC-720, DOC-721).

// Meaning is what a signature states, with a description shown when signing.
type Meaning struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// BuiltIn marks one of the meanings every space has. The screens translate those by
	// key; a meaning a space adds keeps its own words.
	BuiltIn bool `json:"built_in"`
}

// The meanings every space has (DOC-721).
const (
	MeaningReviewed = "reviewed"
	MeaningApproved = "approved"
	MeaningReleased = "released"
	MeaningVerified = "verified"
)

// BuiltInMeanings returns fresh copies of the four meanings every space has. Their words are
// those a record keeps when the screens do not translate them.
func BuiltInMeanings() []Meaning {
	return []Meaning{
		{Key: MeaningReviewed, Name: "Reviewed", Description: "You checked the content of this version and found it correct.", BuiltIn: true},
		{Key: MeaningApproved, Name: "Approved", Description: "You approve this version for use.", BuiltIn: true},
		{Key: MeaningReleased, Name: "Released", Description: "You release this version, so it takes effect.", BuiltIn: true},
		{Key: MeaningVerified, Name: "Verified", Description: "You verified that this version matches what it describes.", BuiltIn: true},
	}
}

// BuiltInMeaning finds one of the meanings every space has.
func BuiltInMeaning(key string) (Meaning, bool) {
	for _, m := range BuiltInMeanings() {
		if m.Key == key {
			return m, true
		}
	}
	return Meaning{}, false
}

// MeaningKeyPattern is what a meaning's key looks like: the same as the ids of states.
var MeaningKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

// SignatureMeanings lists the meanings a definition names, once each, in order.
func (d Definition) SignatureMeanings() []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range d.Transitions {
		if t.SignatureMeaning != "" && !seen[t.SignatureMeaning] {
			seen[t.SignatureMeaning] = true
			out = append(out, t.SignatureMeaning)
		}
	}
	return out
}

// checkSignature checks the signature a transition requires. A request for changes is never
// signed: a signature states agreement with a version, and "changes" states the opposite.
func checkSignature(tr Transition) *Problem {
	if tr.SignatureMeaning == "" {
		return nil
	}
	if !MeaningKeyPattern.MatchString(tr.SignatureMeaning) {
		return problem("bad_meaning", tr.ID, "The step %q requires a signature with a meaning that does not exist.", tr.Name)
	}
	if tr.Outcome == OutcomeChanges {
		return problem("signature_on_changes", tr.ID, "The step %q follows a request for changes, and nobody signs that.", tr.Name)
	}
	return nil
}
