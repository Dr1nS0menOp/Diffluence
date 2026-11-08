// SPDX-License-Identifier: Apache-2.0

// Package plaintext cleans short texts that people give as names: titles, file names,
// display names and the names of spaces.
package plaintext

import "strings"

// directionMarks are the Unicode characters that change the order in which text is read.
// In a name they can make `invoice\u202efdp.exe` look like `invoiceexe.pdf` (SEC-319).
// Joiners and other format characters stay, because emoji and some scripts need them.
var directionMarks = strings.NewReplacer(
	"\u061c", "", // Arabic letter mark
	"\u200e", "", // left-to-right mark
	"\u200f", "", // right-to-left mark
	"\u202a", "", // left-to-right embedding
	"\u202b", "", // right-to-left embedding
	"\u202c", "", // pop directional formatting
	"\u202d", "", // left-to-right override
	"\u202e", "", // right-to-left override
	"\u2066", "", // left-to-right isolate
	"\u2067", "", // right-to-left isolate
	"\u2068", "", // first strong isolate
	"\u2069", "", // pop directional isolate
)

// WithoutDirectionMarks removes the characters that change reading order. A name shows
// in its own direction anyway, because the screen isolates every name it shows.
func WithoutDirectionMarks(name string) string {
	return directionMarks.Replace(name)
}
