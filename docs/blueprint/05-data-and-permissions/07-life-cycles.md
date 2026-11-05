# 05.7: Life cycles

## 7.1 A page

```
             publish                 delete                 retention or purge
  draft ──────────────► published ───────────► trashed ─────────────────────► gone
    │                       │  ▲                  │
    │ delete                │  │ unarchive        │ restore (parent not in the trash)
    └──────────────► trashed│  │                  └─► back to what it was
                            ▼  │
                         archived (wave D: published, with archived_at)
```

- **DAT-160** · built · Unpublished: only its creator reaches it. No child page can be made below it.
- **DAT-161** · built · Publishing writes a version, and in the same transaction the search entry, the links, the audit event and the outbox event, and removes the person's draft.
- **DAT-162** · built · Delete takes the branch along. It is refused when the person cannot delete every page in it. Pages that were in the trash already keep their own item.
- **DAT-163** · built · Restore is refused while the parent is in the trash, so a page never loses a restriction it inherited.
- **DAT-164** · built · Purge takes the branch, also pages below that were trashed earlier. Versions, drafts, restrictions, favourites, search entries, links from it and files go with it. Audit rows stay. `page_links` rows that point at it stay, so old links can be found.
- **DAT-165** · built · The person who deleted a page can undo it for 60 seconds, without *administer*.
- **DAT-166** · wave D · Archive keeps the page in `page_tree`, so it keeps what it inherits. A page below that the person may not archive moves up, under the rule of a move. A page below a page that is still archived cannot be restored alone (ADR-026).
- **DAT-167** · wave D · Moving to another space takes the branch, its files and its history. It is refused while the branch holds something the person cannot see. A page in the trash counts as that today, so a branch with a trashed page cannot move until it is purged or restored.

## 7.2 A space

- **DAT-168** · built · Active, archived, trashed, purged. Archived is read-only for everyone, by rule. In a trashed space nothing works except through the trash, and only administrators reach that.
- **DAT-169** · built · Purge deletes the row, and the cascade takes everything with a `space_id`. Audit rows stay.

## 7.3 What deleting a page does to what points at it

| What | Page in the trash | Page purged |
|---|---|---|
| Links from other pages | Link shows as hidden or deleted (wave E), row stays | Row stays, target gone |
| Favourites, recent | Not listed, row stays | Cascade |
| Watches | No events, row stays | Must go with the page (wave C) |
| Notifications | Hidden at show time | Hidden, expire |
| Comments, reactions, tasks | Unreachable | Cascade |
| Includes (wave E) | Shows nothing | Shows nothing |
| Public site (wave G) | Leaves in a build that starts at once | Same |
| Attachments | Unreachable, stay active | Queued for deletion |
| Header image, attachment in another page's document | The other page shows a gap | Same |

## 7.4 What goes along, and what stays

| Object | Copy of a page | Move in a space | Move to a space | Version restore | Export (own view) | Bundle |
|---|---|---|---|---|---|---|
| Text, title | Yes | Yes | Yes | Text and title of that version | Published only | All versions |
| Versions | No | Yes | Yes | New version | On request | Yes |
| Own restrictions | Yes, unless turned off | Yes | Yes | Untouched | No | Yes |
| Inherited restrictions | **Lost** (GAP-003) | Needs *manage restrictions* when lost | Same | Untouched | No | Yes |
| Files | Copied, ids remapped | Yes | Yes | Untouched | On request | Yes |
| Labels | On request | Yes | Yes | Untouched | No | Yes |
| Comments, reactions | Never | Yes | Yes | Untouched | On request | Yes |
| Favourites, watches | Never | Yes | Yes, for who can still see it | Untouched | No | No |
| Page look | No, a copy starts plain | Yes | Yes | Untouched | No | Yes |
| Owner | The person copying | Yes | Yes | Untouched | No | Yes |

## 7.5 A person

- **DAT-170** · built · Active and disabled. Disabling ends every session and stops every token. Content stays. A person can be enabled again.
- **DAT-171** · wave F · Expired: the date on the account makes it inactive at once, by rule, before any job runs.
- **DAT-172** · wave F · Anonymised: for good. See 05.1, section 1.4.
- **DAT-173** · wave F · When a person is disabled, their personal space stays. An administrator hands it over or archives it.
