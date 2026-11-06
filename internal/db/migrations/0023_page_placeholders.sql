-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Wave M (spec 023).

-- One row per link to a page that does not exist yet, in a page's current version. The
-- target is the page it became. It is not in the document, because a version never changes.
CREATE TABLE page_placeholders (
    source_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    title      text NOT NULL,
    target_id  uuid REFERENCES pages (id) ON DELETE SET NULL,
    PRIMARY KEY (source_id, title)
);
CREATE INDEX page_placeholders_target_idx ON page_placeholders (target_id);
