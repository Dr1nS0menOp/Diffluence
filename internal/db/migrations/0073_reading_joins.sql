-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Wave RV9: the job of read and understood looks only at people who joined a group since its
-- last pass (DOC-1418), and reading is asked of those who may read the page (DOC-823). The
-- table, the trigger, the index and the function are new; the previous release reads none of
-- them and still finds late joiners its own way, so both run together. Like
-- every migration since 0049 it may run twice, so each statement checks first.

-- A join waiting for the job. The row is written by the membership's own transaction, so it
-- becomes visible exactly when the join does, whatever order transactions commit in; the job
-- takes rows and asks those that a reading request names and who may read its page.
CREATE TABLE IF NOT EXISTS read_joins (
    group_id   uuid NOT NULL,
    user_id    uuid NOT NULL,
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);

-- Every join is noted, not only joins of groups a request names now: a request made while the
-- join is not yet committed cannot see it, and the job would then never learn of it.
CREATE OR REPLACE FUNCTION read_joins_note() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO read_joins (group_id, user_id) VALUES (NEW.group_id, NEW.user_id) ON CONFLICT DO NOTHING;
    RETURN NULL;
END $$;
DROP TRIGGER IF EXISTS group_members_read_joins ON group_members;
CREATE TRIGGER group_members_read_joins AFTER INSERT ON group_members
    FOR EACH ROW EXECUTE FUNCTION read_joins_note();

-- The job finds the requests that name a group through this index, not by reading them all.
-- rewrite-ok: the table came with migration 0071 of this same release and holds one row per approved version that asks for reading
CREATE INDEX IF NOT EXISTS read_requests_groups_idx ON read_requests USING gin (groups);

-- perm_page_readers answers which of these people may view one page: the rule of
-- perm_visible_pages asked the other way round, for one page and many people, so a group of
-- thousands is resolved without working out every page each of them may see. The view
-- restrictions read are those on the page and above it; the control rule is read only for a
-- page that is never approved or withdrawn. permissions.Evaluate is the same rule, and the
-- matrix test holds the two equal.
CREATE OR REPLACE FUNCTION perm_page_readers(subject_page uuid, people uuid[]) RETURNS TABLE (user_id uuid)
    LANGUAGE sql STABLE AS $$
    -- What does not depend on the person is read once, not once per person.
    WITH levels AS MATERIALIZED (
        SELECT r.page_id, r.subject_type, r.subject_id
        FROM page_tree t
        JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
        WHERE t.descendant_id = subject_page
    ), unapproved AS MATERIALIZED (
        SELECT EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = subject_page
                       AND (pc.approved_version IS NULL OR pc.state_kind = 'obsolete')) AS hidden
    )
    SELECT x.id
    FROM unnest(people) AS x (id)
    JOIN pages p ON p.id = subject_page
    CROSS JOIN unapproved
    WHERE p.space_id IN (SELECT ps.space_id FROM perm_spaces(x.id, 'space.view') ps)
      AND (p.status = 'published' OR (p.status = 'draft' AND p.created_by = x.id))
      AND p.kind <> 'template'
      AND NOT EXISTS (
          SELECT 1
          FROM levels l
          GROUP BY l.page_id
          HAVING NOT bool_or(
                 (l.subject_type = 'user' AND l.subject_id = x.id)
              OR (l.subject_type = 'group' AND l.subject_id IN
                     (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = x.id))))
      AND CASE WHEN unapproved.hidden
               THEN subject_page NOT IN (SELECT c.page_id FROM perm_control_hidden_pages(x.id) c)
               ELSE true END
$$;

-- The members of named groups that the previous release's pass would still have looked at: the
-- first pass after the upgrade asks whoever it had not asked yet, once.
INSERT INTO read_joins (group_id, user_id)
SELECT gm.group_id, gm.user_id FROM group_members gm
WHERE EXISTS (SELECT 1 FROM read_requests rq WHERE rq.groups @> ARRAY[gm.group_id])
ON CONFLICT DO NOTHING;
