// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// helperProbeTimeout keeps a check short: a helper that takes longer than this to say it
// is alive counts as not answering.
const helperProbeTimeout = 5 * time.Second

const (
	pdfHealthPath       = "/health"
	collaboraHealthPath = "/hosting/discovery"
)

// HelperCheck is what the worker found when it asked one optional helper whether it
// answers (ARC-072). Detail is a short reason in our own words, never an address or
// what the helper said.
type HelperCheck struct {
	Name   string
	OK     bool
	Detail string
}

// CheckHelpers asks each configured helper whether it answers. A helper that is not
// configured is left out: it is off on purpose, not broken. Nothing here ever runs for
// /readyz: a wiki that cannot make a PDF is still a wiki.
func (s *Service) CheckHelpers(ctx context.Context) []HelperCheck {
	var out []HelperCheck
	if s.pdfURL != "" {
		out = append(out, probeWith(ctx, "pdf", strings.TrimRight(s.pdfURL, "/")+pdfHealthPath, s.askHelper))
	}
	switch {
	case s.collaboraURL != "":
		out = append(out, probeWith(ctx, "office", strings.TrimRight(s.collaboraURL, "/")+collaboraHealthPath, http.DefaultClient.Do))
	case s.pdfURL != "":
		// Office files go through the LibreOffice of the PDF helper, so they share its state.
		pdf := out[0]
		out = append(out, HelperCheck{Name: "office", OK: pdf.OK, Detail: pdf.Detail})
	}
	return out
}

// ProbeURL asks one address whether it answers with 200, within the probe timeout.
func ProbeURL(ctx context.Context, name, address string) HelperCheck {
	return probeWith(ctx, name, address, http.DefaultClient.Do)
}

func probeWith(ctx context.Context, name, address string, do func(*http.Request) (*http.Response, error)) HelperCheck {
	ctx, cancel := context.WithTimeout(ctx, helperProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return HelperCheck{Name: name, Detail: "The address of this helper is not valid."}
	}
	resp, err := do(req)
	if err != nil {
		return HelperCheck{Name: name, Detail: "The helper did not answer."}
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HelperCheck{Name: name, Detail: "The helper answered with an error."}
	}
	return HelperCheck{Name: name, OK: true}
}
