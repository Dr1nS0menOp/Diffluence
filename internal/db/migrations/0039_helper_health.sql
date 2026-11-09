-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 039. helper_health keeps what the worker found when it last asked each optional
-- helper whether it answers (ARC-072). /readyz never asks a helper: a wiki that cannot
-- make a PDF is still a wiki. The previous release ignores this table.

CREATE TABLE helper_health (
    name       text PRIMARY KEY CHECK (name IN ('pdf', 'office', 'drawing', 'collaboration')),
    ok         boolean NOT NULL,
    -- A short reason in our own words, never an address or an answer of the helper.
    detail     text NOT NULL DEFAULT '' CHECK (length(detail) <= 200),
    checked_at timestamptz NOT NULL DEFAULT now()
);
