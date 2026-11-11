-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 051, OPS-103. The size of the database, measured once an hour by one worker, so
-- the health page can say how fast it grew in 30 days without reading every file of the
-- database on each look. Rows older than 40 days are removed daily. The previous release
-- ignores this table.

CREATE TABLE database_size_samples (
    taken_at timestamptz PRIMARY KEY DEFAULT now(),
    bytes    bigint NOT NULL CHECK (bytes >= 0)
);
