-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 075, wave CS1. The comparison of the audit chain with its witness runs outside, next to
-- the log store, and reports its result here with "diffluence audit verify --witness --report",
-- so the console can show the newest result and its age (CSL-532). The application role may
-- add a report and read them; it may not change or remove one. Each node also says the log
-- level it runs at (CSL-572), and, from wave CS5, which key ids it holds (CSL-551). The
-- previous release ignores all of it. It may run twice, as every migration since 0049 may.

CREATE TABLE IF NOT EXISTS audit_witness_checks (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    checked_at    timestamptz NOT NULL DEFAULT now(),
    ok            boolean NOT NULL,
    compared      integer NOT NULL CHECK (compared >= 0),
    pruned        integer NOT NULL CHECK (pruned >= 0),
    first_bad_seq bigint NOT NULL DEFAULT 0 CHECK (first_bad_seq >= 0),
    reason        text NOT NULL DEFAULT '' CHECK (length(reason) <= 300),
    CHECK (NOT ok OR (first_bad_seq = 0 AND reason = ''))
);
CREATE INDEX IF NOT EXISTS audit_witness_checks_newest ON audit_witness_checks (checked_at DESC);

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS log_level text NOT NULL DEFAULT '' CHECK (log_level IN ('', 'debug', 'info', 'warn', 'error')),
    ADD COLUMN IF NOT EXISTS key_ids text[] NOT NULL DEFAULT '{}';
