// SPDX-License-Identifier: Apache-2.0

package db

// tableOwners names the module that owns each table the migrations create (ARC-027, the
// table in docs/blueprint/04-architecture/02-modules.md). A table of the installation as a
// whole, written by the wiring itself, belongs to internal/app.
var tableOwners = map[string]string{
	// identity: people, groups, sessions and the ways to log in.
	"users":                "internal/identity",
	"local_credentials":    "internal/identity",
	"sessions":             "internal/identity",
	"global_grants":        "internal/identity",
	"groups":               "internal/identity",
	"group_members":        "internal/identity",
	"group_owners":         "internal/identity",
	"totp_credentials":     "internal/identity",
	"recovery_codes":       "internal/identity",
	"webauthn_credentials": "internal/identity",
	"auth_challenges":      "internal/identity",
	"user_preferences":     "internal/identity",
	"api_tokens":           "internal/identity",
	"login_history":        "internal/identity",
	"known_devices":        "internal/identity",
	"user_identities":      "internal/sso",
	"scim_tokens":          "internal/scim",
	"user_profiles":        "internal/people",
	"setup_codes":          "internal/setup",

	// permissions: the access rule and what it reads that no other module writes.
	"space_grants":          "internal/permissions",
	"page_restrictions":     "internal/permissions",
	"roles":                 "internal/permissions",
	"role_assignments":      "internal/permissions",
	"permission_sets":       "internal/permissions",
	"permission_generation": "internal/permissions",

	// audit and events.
	"audit_events":           "internal/audit",
	"audit_checkpoints":      "internal/audit",
	"audit_anchors":          "internal/audit",
	"audit_personal":         "internal/audit",
	"audit_pending":          "internal/audit",
	"audit_signing_keys":     "internal/audit",
	"audit_erasure_baseline": "internal/audit",
	"audit_entries":          "internal/audit",
	"outbox_events":          "internal/outbox",
	"event_failures":         "internal/failures",
	"event_head":             "internal/events",
	"event_transport":        "internal/events",
	"event_log":              "internal/events",
	"event_cursors":          "internal/events",
	"event_retries":          "internal/events",

	// spaces.
	"spaces":               "internal/spaces",
	"space_shortcuts":      "internal/spaces",
	"space_favourites":     "internal/spaces",
	"space_owners":         "internal/spaces",
	"space_key_aliases":    "internal/spaces",
	"space_categories":     "internal/spaces",
	"space_category_links": "internal/spaces",
	"space_features":       "internal/features",

	// pages: the tree, versions, drafts, labels, approval and document control.
	"pages":                    "internal/pages",
	"page_tree":                "internal/pages",
	"page_versions":            "internal/pages",
	"drafts":                   "internal/pages",
	"favourites":               "internal/pages",
	"recent_pages":             "internal/pages",
	"page_links":               "internal/pages",
	"page_labels":              "internal/pages",
	"page_mentions":            "internal/pages",
	"page_facts":               "internal/pages",
	"page_decisions":           "internal/pages",
	"page_includes":            "internal/pages",
	"page_placeholders":        "internal/pages",
	"page_verifications":       "internal/pages",
	"scheduled_publications":   "internal/pages",
	"removed_versions":         "internal/pages",
	"space_fixed_labels":       "internal/pages",
	"space_stages":             "internal/pages",
	"space_approvers":          "internal/pages",
	"approval_rounds":          "internal/pages",
	"approval_answers":         "internal/pages",
	"page_control":             "internal/pages",
	"workflows":                "internal/pages",
	"workflow_versions":        "internal/pages",
	"space_workflows":          "internal/pages",
	"page_workflows":           "internal/pages",
	"workflow_rounds":          "internal/pages",
	"workflow_answers":         "internal/pages",
	"workflow_reviewers":       "internal/pages",
	"workflow_review_comments": "internal/pages",
	"workflow_templates":       "internal/pages",

	// files.
	"attachments":         "internal/attachments",
	"attachment_versions": "internal/attachments",
	"attachment_labels":   "internal/attachments",
	"attachment_refs":     "internal/attachments",
	"attachment_events":   "internal/attachments",
	"storage_deletions":   "internal/storage",

	// working together.
	"comments":             "internal/comments",
	"comment_seen":         "internal/comments",
	"reactions":            "internal/comments",
	"watches":              "internal/watches",
	"follows":              "internal/watches",
	"page_tasks":           "internal/tasks",
	"notifications":        "internal/notify",
	"mail_queue":           "internal/mail",
	"access_requests":      "internal/sharing",
	"invitation_requests":  "internal/sharing",
	"page_presence":        "internal/presence",
	"page_version_authors": "internal/collab",
	"collab_documents":     "internal/collab",
	"saved_blocks":         "internal/savedblocks",

	// templates, search, trash.
	"templates":           "internal/templates",
	"template_favourites": "internal/templates",
	"template_uses":       "internal/templates",
	"space_templates":     "internal/templates",
	"search_documents":    "internal/search",
	"search_pages":        "internal/search",
	"trash_items":         "internal/trash",

	// exchange and public sites.
	"export_runs":        "internal/exchange",
	"export_run_pages":   "internal/exchange",
	"export_policy":      "internal/exchange",
	"space_pdf_design":   "internal/exchange",
	"import_runs":        "internal/exchange",
	"import_items":       "internal/exchange",
	"public_sites":       "internal/sites",
	"public_site_pages":  "internal/sites",
	"public_site_builds": "internal/sites",
	"public_links":       "internal/sites",

	// platform: analytics, automation, webhooks, outward calls.
	"page_view_days":     "internal/analytics",
	"analytics_salts":    "internal/analytics",
	"page_view_seen":     "internal/analytics",
	"page_view_people":   "internal/analytics",
	"page_view_links":    "internal/analytics",
	"analytics_days":     "internal/analytics",
	"search_terms":       "internal/analytics",
	"automation_rules":   "internal/automation",
	"automation_runs":    "internal/automation",
	"automation_hooks":   "internal/automation",
	"outgoing_hosts":     "internal/outward",
	"webhook_endpoints":  "internal/webhooks",
	"webhook_deliveries": "internal/webhooks",

	// the installation: jobs, settings, nodes, backups and restore.
	"instance":              "internal/instance",
	"jobs":                  "internal/jobs",
	"job_leases":            "internal/jobs",
	"settings":              "internal/settings",
	"nodes":                 "internal/app",
	"storage_state":         "internal/app",
	"helper_health":         "internal/app",
	"database_size_samples": "internal/app",
	"backup_requests":       "internal/backups",
	"backup_runs":           "internal/backups",
	"backup_restore_checks": "internal/backups",
	"backup_mail":           "internal/backups",
	"backup_worker":         "internal/backups",
	"ops.restores":          "internal/restore",
	"ops.nodes":             "internal/restore",
}

// crossings lists, per module and table, how often a module names a table it does not own.
// It is the state found on 2026-11-13 (spec 072), less what moved behind the owner then. It
// may only shrink: a new mention calls the owner's Go API instead, and a mention that is
// truly needed comes with its reason. Most are what ARC-027 allows, a list that joins a
// display column such as a title or a name, because one join is one query and a call per
// row would be hundreds; the rest are writes spec 032 records, done in one transaction.
var crossings = map[crossing]int{
	// analytics: the lists of what is read show titles, spaces and names, and count the
	// comments and mentions of a page.
	{"internal/analytics", "comments"}:      3,
	{"internal/analytics", "page_mentions"}: 1,
	{"internal/analytics", "page_versions"}: 1,
	{"internal/analytics", "pages"}:         10,
	{"internal/analytics", "spaces"}:        1,
	{"internal/analytics", "users"}:         3,

	// app: the wiring runs the installation's upkeep (pruning, the queues, the waiting page,
	// key rotation and the checks at start), which spans the tables of many modules.
	{"internal/app", "audit_events"}:          3,
	{"internal/app", "audit_pending"}:         2,
	{"internal/app", "backup_requests"}:       1,
	{"internal/app", "backup_restore_checks"}: 2,
	{"internal/app", "backup_runs"}:           2,
	{"internal/app", "event_failures"}:        1,
	{"internal/app", "instance"}:              1,
	{"internal/app", "jobs"}:                  7,
	{"internal/app", "local_credentials"}:     1,
	{"internal/app", "mail_queue"}:            6,
	{"internal/app", "outbox_events"}:         4,
	{"internal/app", "sessions"}:              1,
	{"internal/app", "storage_deletions"}:     2,
	{"internal/app", "totp_credentials"}:      2,
	{"internal/app", "trash_items"}:           1,
	{"internal/app", "webhook_endpoints"}:     4,

	// attachments: file lists show the page, space and uploader; files hang on drafts and
	// comments; removed files are queued for deletion in the same transaction (spec 032).
	{"internal/attachments", "comments"}:          1,
	{"internal/attachments", "drafts"}:            1,
	{"internal/attachments", "pages"}:             6,
	{"internal/attachments", "spaces"}:            4,
	{"internal/attachments", "storage_deletions"}: 5,
	{"internal/attachments", "users"}:             5,

	// automation: a rule matches pages, labels, groups and grants, and its runs name people.
	{"internal/automation", "group_members"}: 2,
	{"internal/automation", "groups"}:        1,
	{"internal/automation", "page_labels"}:   1,
	{"internal/automation", "page_tree"}:     1,
	{"internal/automation", "pages"}:         9,
	{"internal/automation", "space_grants"}:  1,
	{"internal/automation", "spaces"}:        2,
	{"internal/automation", "users"}:         5,

	// backups: a failed backup is mailed to every active administrator.
	{"internal/backups", "users"}: 1,

	// blocks: a dynamic block is one query over pages, labels, tasks, files and people (wave E).
	{"internal/blocks", "attachment_labels"}:    2,
	{"internal/blocks", "attachment_versions"}:  2,
	{"internal/blocks", "attachments"}:          2,
	{"internal/blocks", "comments"}:             3,
	{"internal/blocks", "drafts"}:               1,
	{"internal/blocks", "favourites"}:           1,
	{"internal/blocks", "follows"}:              1,
	{"internal/blocks", "groups"}:               1,
	{"internal/blocks", "page_decisions"}:       1,
	{"internal/blocks", "page_facts"}:           1,
	{"internal/blocks", "page_labels"}:          11,
	{"internal/blocks", "page_mentions"}:        1,
	{"internal/blocks", "page_tasks"}:           2,
	{"internal/blocks", "page_tree"}:            4,
	{"internal/blocks", "page_versions"}:        5,
	{"internal/blocks", "pages"}:                19,
	{"internal/blocks", "reactions"}:            1,
	{"internal/blocks", "search_documents"}:     2,
	{"internal/blocks", "space_categories"}:     1,
	{"internal/blocks", "space_category_links"}: 1,
	{"internal/blocks", "space_favourites"}:     2,
	{"internal/blocks", "spaces"}:               5,
	{"internal/blocks", "users"}:                16,
	{"internal/blocks", "watches"}:              1,

	// collab: the authors of a version are recorded only for a page that exists, and named.
	{"internal/collab", "pages"}: 1,
	{"internal/collab", "users"}: 1,

	// comments: files named in a comment must be on the page, and the space of the page is
	// audited; pages imports comments, so comments cannot call pages for these (ARC-036).
	{"internal/comments", "attachments"}: 1,
	{"internal/comments", "pages"}:       3,
	{"internal/comments", "users"}:       3,

	// console: the administration overview counts and lists across the installation.
	{"internal/console", "api_tokens"}:          1,
	{"internal/console", "attachment_versions"}: 5,
	{"internal/console", "attachments"}:         4,
	{"internal/console", "audit_entries"}:       2,
	{"internal/console", "event_failures"}:      1,
	{"internal/console", "helper_health"}:       1,
	{"internal/console", "invitation_requests"}: 1,
	{"internal/console", "jobs"}:                2,
	{"internal/console", "nodes"}:               1,
	{"internal/console", "outbox_events"}:       1,
	{"internal/console", "page_restrictions"}:   2,
	{"internal/console", "page_versions"}:       1,
	{"internal/console", "pages"}:               5,
	{"internal/console", "scim_tokens"}:         1,
	{"internal/console", "settings"}:            1,
	{"internal/console", "spaces"}:              6,
	{"internal/console", "storage_deletions"}:   1,
	{"internal/console", "user_profiles"}:       1,
	{"internal/console", "users"}:               5,

	// events: the event path beside NATS reads the outbox it relays and the live nodes. The
	// switch reads which releases the live nodes run (internal/app imports events, so its
	// registry has no API events could call), and the check of the release without NATS looks
	// for an event published to NATS after the drain (spec 068, wave EV2).
	{"internal/events", "nodes"}:         3,
	{"internal/events", "outbox_events"}: 5,

	// exchange: an export reads a whole space, and an import restores one in a single
	// transaction (spec 032).
	{"internal/exchange", "attachment_versions"}: 1,
	{"internal/exchange", "comments"}:            4,
	{"internal/exchange", "group_members"}:       1,
	{"internal/exchange", "groups"}:              2,
	{"internal/exchange", "jobs"}:                1,
	{"internal/exchange", "page_labels"}:         2,
	{"internal/exchange", "page_restrictions"}:   4,
	{"internal/exchange", "page_tree"}:           5,
	{"internal/exchange", "page_versions"}:       1,
	{"internal/exchange", "pages"}:               12,
	{"internal/exchange", "space_grants"}:        2,
	{"internal/exchange", "spaces"}:              6,
	{"internal/exchange", "users"}:               4,

	// feed: the start screen joins what changed with who watches and reacted.
	{"internal/feed", "comments"}:         2,
	{"internal/feed", "follows"}:          2,
	{"internal/feed", "page_tree"}:        2,
	{"internal/feed", "page_versions"}:    4,
	{"internal/feed", "pages"}:            5,
	{"internal/feed", "reactions"}:        2,
	{"internal/feed", "search_documents"}: 2,
	{"internal/feed", "spaces"}:           2,
	{"internal/feed", "users"}:            2,
	{"internal/feed", "watches"}:          4,

	// identity: erasing a person removes their rows, and a deleted group its grants, in one
	// transaction (spec 032); the list of a person's spaces shows names.
	{"internal/identity", "access_requests"}:     1,
	{"internal/identity", "follows"}:             1,
	{"internal/identity", "invitation_requests"}: 1,
	{"internal/identity", "notifications"}:       1,
	{"internal/identity", "pages"}:               1,
	{"internal/identity", "role_assignments"}:    1,
	{"internal/identity", "space_grants"}:        1,
	{"internal/identity", "spaces"}:              3,

	// labels: whether the fixed list of a space allows a label; labels sits below pages and
	// spaces, which both import it.
	{"internal/labels", "space_fixed_labels"}: 1,
	{"internal/labels", "spaces"}:             1,

	// notify: who is told, and the words of the notice.
	{"internal/notify", "comments"}:         1,
	{"internal/notify", "group_members"}:    1,
	{"internal/notify", "pages"}:            3,
	{"internal/notify", "spaces"}:           1,
	{"internal/notify", "user_preferences"}: 2,
	{"internal/notify", "users"}:            2,

	// pages: lists show spaces and people; a move or copy carries restrictions and trash
	// rows, and space settings for approval live on spaces (spec 032).
	{"internal/pages", "attachments"}:       3,
	{"internal/pages", "comments"}:          2,
	{"internal/pages", "page_restrictions"}: 8,
	{"internal/pages", "spaces"}:            27,
	{"internal/pages", "trash_items"}:       1,
	{"internal/pages", "users"}:             31,
	// Document control resolves the people, groups and roles a review state names when a
	// round starts, inside the transaction of the step, and names them on the panel and in
	// the editor (spec 064, DOC-200, DOC-201).
	{"internal/pages", "group_members"}:    2,
	{"internal/pages", "groups"}:           4,
	{"internal/pages", "role_assignments"}: 2,
	{"internal/pages", "roles"}:            4,

	// people: the directory shows people and their groups.
	{"internal/people", "group_members"}: 1,
	{"internal/people", "users"}:         2,

	// permissions: the access rule reads the facts it decides from.
	{"internal/permissions", "global_grants"}: 3,
	{"internal/permissions", "group_members"}: 6,
	{"internal/permissions", "groups"}:        7,
	{"internal/permissions", "page_control"}:  2,
	// Being asked to review opens a page for the round only, so the rule reads the open
	// round and its reviewers as a fact (spec 064, DOC-203).
	{"internal/permissions", "workflow_reviewers"}: 1,
	{"internal/permissions", "workflow_rounds"}:    1,
	{"internal/permissions", "page_tree"}:          3,
	{"internal/permissions", "pages"}:              3,
	{"internal/permissions", "spaces"}:             5,
	{"internal/permissions", "users"}:              13,

	// presence: one stamp of what changed on a page, read on every poll.
	{"internal/presence", "comments"}:    3,
	{"internal/presence", "page_labels"}: 1,
	{"internal/presence", "pages"}:       1,
	{"internal/presence", "users"}:       1,

	// savedblocks: a shared block is offered to members only.
	{"internal/savedblocks", "users"}: 1,

	// search: the index and its answers join pages, spaces, labels, files and people.
	{"internal/search", "attachment_labels"}: 2,
	{"internal/search", "attachments"}:       2,
	{"internal/search", "comments"}:          1,
	{"internal/search", "groups"}:            2,
	{"internal/search", "page_labels"}:       2,
	{"internal/search", "page_mentions"}:     2,
	{"internal/search", "page_tree"}:         3,
	{"internal/search", "page_versions"}:     2,
	{"internal/search", "pages"}:             5,
	{"internal/search", "spaces"}:            5,
	{"internal/search", "users"}:             4,

	// setup: the first setup stays open only while no administrator exists, read in the
	// transaction that uses the code.
	{"internal/setup", "global_grants"}: 1,

	// sharing: who a page can be shared with, and the space of a page.
	{"internal/sharing", "group_members"}:     2,
	{"internal/sharing", "groups"}:            2,
	{"internal/sharing", "page_restrictions"}: 1,
	{"internal/sharing", "page_tree"}:         1,
	{"internal/sharing", "pages"}:             2,
	{"internal/sharing", "space_grants"}:      3,
	{"internal/sharing", "spaces"}:            1,
	{"internal/sharing", "users"}:             5,

	// sites: a public site is built from published pages without restrictions.
	{"internal/sites", "page_control"}:      2,
	{"internal/sites", "page_restrictions"}: 2,
	{"internal/sites", "page_tree"}:         3,
	{"internal/sites", "page_versions"}:     1,
	{"internal/sites", "pages"}:             6,
	{"internal/sites", "spaces"}:            1,
	{"internal/sites", "users"}:             2,

	// spaces: lists of spaces show people; a deleted space queues its files (spec 032).
	{"internal/spaces", "attachment_versions"}: 1,
	{"internal/spaces", "attachments"}:         1,
	{"internal/spaces", "group_members"}:       1,
	{"internal/spaces", "pages"}:               1,
	{"internal/spaces", "space_grants"}:        1,
	{"internal/spaces", "storage_deletions"}:   1,
	{"internal/spaces", "users"}:               5,

	// sso: a login through a provider finds or links the account.
	{"internal/sso", "local_credentials"}: 2,
	{"internal/sso", "users"}:             2,

	// tasks: the task list shows the page, space and assignee.
	{"internal/tasks", "pages"}:  1,
	{"internal/tasks", "spaces"}: 1,
	{"internal/tasks", "users"}:  1,

	// templates: a template is a page, listed with it and deleted with it (spec 032).
	{"internal/templates", "pages"}: 2,

	// trash: the list shows what each item was and who removed it.
	{"internal/trash", "attachments"}: 2,
	{"internal/trash", "comments"}:    2,
	{"internal/trash", "page_tree"}:   1,
	{"internal/trash", "users"}:       1,

	// watches: who watches what, with names.
	{"internal/watches", "page_tree"}: 2,
	{"internal/watches", "pages"}:     3,
	{"internal/watches", "spaces"}:    2,
	{"internal/watches", "users"}:     5,
}
