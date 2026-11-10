-- SPDX-License-Identifier: Apache-2.0
-- Fills an empty, migrated database with data of the size the stress test used
-- (docs/reports/2026-11-10-stress-test.md): 5,000 people, 300 spaces, 60,000 pages with
-- about 115,000 versions, 30,000 comments, 12,000 files and 250,000 notifications. It
-- writes the tables directly, so it takes a minute instead of hours through the API. It
-- is for EXPLAIN and timing on one machine (spec 050), never for a real installation.
-- Run it as the owner of the tables: psql -v ON_ERROR_STOP=1 -f bench-data.sql

\set people 5000
\set groups 40
\set spaces 300
\set open_spaces 250
\set pages_per_space 200

BEGIN;

CREATE TEMP TABLE bench_instance AS SELECT COALESCE((SELECT id FROM instance LIMIT 1), gen_random_uuid()) AS id;

INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
SELECT ('00000000-0000-4000-8000-' || lpad(to_hex(n), 12, '0'))::uuid, (SELECT id FROM bench_instance),
       'member', 'active', 'person' || n || '@example.test', 'Person ' || n
FROM generate_series(1, :people) n;

INSERT INTO global_grants (subject_type, subject_id, permission)
SELECT 'user', u.id, 'use_product' FROM users u;

INSERT INTO groups (id, origin_instance_id, name, source)
SELECT ('00000000-0000-4000-9000-' || lpad(to_hex(n), 12, '0'))::uuid, (SELECT id FROM bench_instance), 'Group ' || n, 'local'
FROM generate_series(1, :groups) n;

-- Every person is in one group, by number.
INSERT INTO group_members (group_id, user_id)
SELECT ('00000000-0000-4000-9000-' || lpad(to_hex(1 + (n % :groups)), 12, '0'))::uuid,
       ('00000000-0000-4000-8000-' || lpad(to_hex(n), 12, '0'))::uuid
FROM generate_series(1, :people) n;

INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
SELECT ('00000000-0000-4000-a000-' || lpad(to_hex(n), 12, '0'))::uuid, (SELECT id FROM bench_instance),
       'S' || n, 'Space ' || n, 'active', '00000000-0000-4000-8000-000000000001'
FROM generate_series(1, :spaces) n;

-- Most spaces are open to all members; the rest to two groups each.
INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
SELECT s.id, 'all_members', '00000000-0000-0000-0000-000000000000', p
FROM spaces s, unnest(ARRAY['space.view', 'page.create', 'page.edit', 'comment.create']) p
WHERE s.key ~ '^S[0-9]+$' AND substr(s.key, 2)::int <= :open_spaces;
INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
SELECT s.id, 'group', ('00000000-0000-4000-9000-' || lpad(to_hex(1 + ((substr(s.key, 2)::int + g) % :groups)), 12, '0'))::uuid, p
FROM spaces s, generate_series(0, 1) g, unnest(ARRAY['space.view', 'page.create', 'page.edit', 'comment.create']) p
WHERE s.key ~ '^S[0-9]+$' AND substr(s.key, 2)::int > :open_spaces;

INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by, created_at, updated_at)
SELECT ('00000000-0000-4000-b000-' || lpad(to_hex(n), 12, '0'))::uuid, (SELECT id FROM bench_instance),
       ('00000000-0000-4000-a000-' || lpad(to_hex(1 + (n % :spaces)), 12, '0'))::uuid,
       'Page ' || n || ' about ' || (ARRAY['onboarding', 'release', 'budget', 'holiday', 'security', 'roadmap'])[1 + n % 6],
       'published',
       ('00000000-0000-4000-8000-' || lpad(to_hex(1 + (n % :people)), 12, '0'))::uuid,
       ('00000000-0000-4000-8000-' || lpad(to_hex(1 + (n % :people)), 12, '0'))::uuid,
       now() - make_interval(mins => n), now() - make_interval(mins => n)
FROM generate_series(1, :spaces * :pages_per_space) n;
-- A tree in each space: four children a page, about seven levels deep.
UPDATE pages c SET parent_id = p.id
FROM pages p
WHERE c.title LIKE 'Page %'
  AND ('x' || substr(c.id::text, 25))::bit(48)::bigint / :spaces > 0
  AND p.id = ('00000000-0000-4000-b000-' || lpad(to_hex(
        ((('x' || substr(c.id::text, 25))::bit(48)::bigint / :spaces - 1) / 4) * :spaces
        + ('x' || substr(c.id::text, 25))::bit(48)::bigint % :spaces), 12, '0'))::uuid;

INSERT INTO page_tree (ancestor_id, descendant_id, depth)
WITH RECURSIVE up (descendant_id, ancestor_id, depth) AS (
	SELECT id, id, 0 FROM pages WHERE title LIKE 'Page %'
	UNION ALL
	SELECT up.descendant_id, p.parent_id, up.depth + 1
	FROM up JOIN pages p ON p.id = up.ancestor_id
	WHERE p.parent_id IS NOT NULL)
SELECT ancestor_id, descendant_id, depth FROM up;

-- Statistics inside the transaction, so the joins below get a plan for the real sizes.
ANALYZE users, groups, group_members, spaces, space_grants, pages, page_tree;

-- One to three versions a page; about one in twenty is silent.
INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id, silent, created_at)
SELECT gen_random_uuid(), p.origin_instance_id, p.id, v, p.title, '{"type":"doc","content":[]}', 1,
       ('00000000-0000-4000-8000-' || lpad(to_hex(1 + ((('x' || substr(p.id::text, 25))::bit(48)::bigint + v) % :people)), 12, '0'))::uuid,
       random() < 0.05, p.created_at + make_interval(secs => v * 37)
FROM pages p, generate_series(1, 1 + (abs(hashtext(p.id::text)) % 3)) v
WHERE p.title LIKE 'Page %';

INSERT INTO search_documents (page_id, title, body)
SELECT p.id, p.title, 'Text of ' || p.title || '. ' || repeat('Words people write on a wiki page. ', 20)
FROM pages p WHERE p.title LIKE 'Page %'
ON CONFLICT DO NOTHING;

-- One page in two hundred is restricted to one group.
INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
SELECT p.id, 'view', 'group', ('00000000-0000-4000-9000-' || lpad(to_hex(1 + (abs(hashtext(p.id::text)) % :groups)), 12, '0'))::uuid
FROM pages p WHERE p.title LIKE 'Page %' AND abs(hashtext(p.id::text)) % 200 = 0;
ANALYZE page_versions, search_documents, page_restrictions;

INSERT INTO comments (id, origin_instance_id, space_id, page_id, author_id, body)
SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, p.id,
       ('00000000-0000-4000-8000-' || lpad(to_hex(1 + (n % :people)), 12, '0'))::uuid,
       '{"type":"doc","content":[]}'
FROM generate_series(1, 30000) n
JOIN pages p ON p.id = ('00000000-0000-4000-b000-' || lpad(to_hex(1 + (n * 7 % (:spaces * :pages_per_space))), 12, '0'))::uuid;

INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by)
SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, p.id,
       (ARRAY['report', 'budget', 'holiday-plan', 'onboarding', 'minutes', 'design'])[1 + n % 6] || '-' || n || '.pdf',
       'active', p.created_by
FROM generate_series(1, 12000) n
JOIN pages p ON p.id = ('00000000-0000-4000-b000-' || lpad(to_hex(1 + (n * 5 % (:spaces * :pages_per_space))), 12, '0'))::uuid;

-- Fifty notifications a person, a third of them unread.
INSERT INTO notifications (id, user_id, type, reason, page_id, space_id, group_key, source_event, read_at)
SELECT gen_random_uuid(), u.id, 'page.changed', 'watching', p.id, p.space_id, p.id::text, gen_random_uuid(),
       CASE WHEN k % 3 = 0 THEN NULL ELSE now() END
FROM users u, generate_series(1, 50) k
JOIN LATERAL (SELECT id, space_id FROM pages
              WHERE id = ('00000000-0000-4000-b000-' || lpad(to_hex(1 + ((k * 997) % (:spaces * :pages_per_space))), 12, '0'))::uuid) p ON true
WHERE u.email LIKE 'person%@example.test';

COMMIT;
ANALYZE;
