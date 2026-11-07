// SPDX-License-Identifier: Apache-2.0

package document

import "fmt"

// UpgradeStep turns a stored document of version From into version From+1.
type UpgradeStep struct {
	From int
	Run  func(raw []byte) ([]byte, error)
}

// upgradeSteps is empty while version 1 is the only one. A release that changes the
// format adds its step here and raises SchemaVersion.
var upgradeSteps = []UpgradeStep{}

// Upgrade brings a stored document to the current schema, on reading only: a version
// never changes. A document from a newer release is left as it is (EDT-157, EDT-158).
func Upgrade(raw []byte, version int) ([]byte, error) {
	return upgrade(raw, version, SchemaVersion, upgradeSteps)
}

func upgrade(raw []byte, version, current int, steps []UpgradeStep) ([]byte, error) {
	for from := version; from < current; from++ {
		for _, step := range steps {
			if step.From != from {
				continue
			}
			next, err := step.Run(raw)
			if err != nil {
				return nil, fmt.Errorf("upgrade a document from version %d: %w", from, err)
			}
			raw = next
		}
	}
	return raw, nil
}
