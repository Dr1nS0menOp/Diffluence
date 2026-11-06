-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- What dynamic blocks collect, and more about files (spec 012). All indexes are
-- filled when a page is published, in that transaction.

CREATE TABLE page_facts (
    page_id    uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    facts_id   text NOT NULL DEFAULT '',
    key        text NOT NULL,
    position   integer NOT NULL,
    value      text NOT NULL,
    value_date date,
    value_num  double precision,
    PRIMARY KEY (page_id, position)
);
CREATE INDEX page_facts_key_idx ON page_facts (facts_id, key);

CREATE TABLE page_decisions (
    page_id   uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    position  integer NOT NULL,
    text      text NOT NULL,
    PRIMARY KEY (page_id, position)
);

-- The target has no foreign key: a source that is gone is told to its editors.
CREATE TABLE page_includes (
    source_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    target_id  uuid NOT NULL,
    name       text NOT NULL DEFAULT '',
    PRIMARY KEY (source_id, target_id, name)
);
CREATE INDEX page_includes_target_idx ON page_includes (target_id);

ALTER TABLE attachments ADD COLUMN comment text NOT NULL DEFAULT '';
ALTER TABLE spaces ADD COLUMN storage_limit_mib integer NOT NULL DEFAULT 0;
