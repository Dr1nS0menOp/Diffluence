-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- More about files: labels, who shows them, and what happened to them (spec 024).

CREATE TABLE attachment_labels (
    attachment_id  uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    label          text NOT NULL,
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attachment_id, label)
);
CREATE INDEX attachment_labels_label_idx ON attachment_labels (label);

-- Which version of which page shows a file. Derived from page_versions (DAT-011). The
-- file has no foreign key: a document can still point at a file that was purged.
CREATE TABLE attachment_refs (
    attachment_id  uuid NOT NULL,
    page_id        uuid NOT NULL,
    version        integer NOT NULL,
    PRIMARY KEY (attachment_id, page_id, version),
    FOREIGN KEY (page_id, version) REFERENCES page_versions (page_id, number) ON DELETE CASCADE
);
CREATE INDEX attachment_refs_page_idx ON attachment_refs (page_id, version);

INSERT INTO attachment_refs (attachment_id, page_id, version)
SELECT DISTINCT found.id::uuid, v.page_id, v.number
FROM page_versions v
CROSS JOIN LATERAL (
    SELECT jsonb_path_query(v.document, '$.**.attachment_id') #>> '{}' AS id
) found
WHERE found.id ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$';

-- What the history of a page says about its files. Purging a file takes its lines along.
CREATE TABLE attachment_events (
    id             uuid PRIMARY KEY,
    page_id        uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    attachment_id  uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    filename       text NOT NULL,
    action         text NOT NULL CHECK (action IN ('added', 'version', 'removed', 'restored', 'moved_here', 'moved_away')),
    actor_id       uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attachment_events_page_idx ON attachment_events (page_id, created_at DESC);
