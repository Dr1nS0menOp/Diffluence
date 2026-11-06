// SPDX-License-Identifier: Apache-2.0

package webui

import (
	"strings"
	"testing"
)

func TestVideoAndAudioFromOurOwnOriginMayPlay(t *testing.T) {
	if got := contentPolicy(""); !strings.Contains(got, "media-src 'self'") {
		t.Fatalf("media-src: %s", got)
	}
}

func TestOnlyTheDrawingServiceMayBeFramed(t *testing.T) {
	if got := contentPolicy(""); strings.Contains(got, "frame-src") {
		t.Fatalf("no service, no frames: %s", got)
	}
	got := contentPolicy("https://draw.example.com")
	if !strings.Contains(got, "; frame-src https://draw.example.com;") {
		t.Fatalf("frame-src: %s", got)
	}
	if !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("our own page must stay unframeable: %s", got)
	}
}
