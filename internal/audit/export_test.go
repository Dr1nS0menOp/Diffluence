// SPDX-License-Identifier: Apache-2.0

package audit

import "crypto/ed25519"

// SignedHeadForTest signs a head of the chain as a checkpoint does, so a test can write a
// checkpoint straight into the table.
func SignedHeadForTest(key ed25519.PrivateKey, seq int64, hash []byte) []byte {
	return ed25519.Sign(key, checkpointMessage(seq, hash))
}

// WithExportPageForTest makes an export with f read n events per query.
func WithExportPageForTest(f Filter, n int) Filter {
	f.exportPage = n
	return f
}
