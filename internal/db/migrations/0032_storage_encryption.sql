-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Whether every stored file is encrypted (SEC-725, spec 032). Until it is, files written before
-- encryption are still read as they are; after it, a plain file is refused, so a file swapped
-- for a plain one cannot be served. One row, which the encryption job fills in once.

CREATE TABLE storage_state (
    id                bool PRIMARY KEY DEFAULT true CHECK (id),
    encrypted_all_at  timestamptz
);
INSERT INTO storage_state (id) VALUES (true);