-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 046, ADR-038. Password login stops after 100 wrong passwords in a row on one
-- account (NIST SP 800-63B), and stays stopped across restarts, so the count lives here
-- and not in the memory of one node. Only the password route counts; passkeys and the
-- identity provider never look at these columns. The previous release ignores them.

ALTER TABLE local_credentials
    ADD COLUMN failed_logins_in_row integer NOT NULL DEFAULT 0 CHECK (failed_logins_in_row >= 0),
    ADD COLUMN password_locked_at timestamptz;
