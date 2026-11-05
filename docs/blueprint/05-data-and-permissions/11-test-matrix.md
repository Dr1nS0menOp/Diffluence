# 05.11: The permission matrix as test cases

NIST SP 800-192 asks that a policy written as a model is checked for inconsistency and incompleteness against what its authors meant. OWASP asks for tests that prove deny by default and safe failure. Our form of that: three independent answers that must agree on every generated case.

## 11.1 The three answers

- **PRM-150** · built · `literalRule`, written out by hand in the test, on the case tables and not on production types. `permissions.Evaluate`. The SQL functions on real rows. Any difference fails the build.
- **PRM-151** · built · Every bug found later adds a case. Every new rule adds a dimension, and lands in the same commit in all three places.
- **PRM-152** · wave D · Second paths join the matrix: `mayReadRound`, the `templates` module's rule, `UndoDelete`.
- **PRM-153** · wave E · Named variants of the rule join too, such as "could have seen this page".

## 11.2 Dimensions

Status **built** means it is in `matrix_test.go` today.

| Number | Status | Dimension | Values |
|---|---|---|---|
| **PRM-154** | built | Person | member, member in group, guest, guest in group, disabled, without `use_product` |
| **PRM-155** | new | Person, missing | **service**, `use_product` through a group, `view_profiles` on and off, in two groups |
| **PRM-156** | wave F | Person, later | expired, expiring tomorrow, anonymised |
| **PRM-157** | built | Grant | none, user viewer, editor, admin, group viewer, editor, all members viewer, editor, all guests viewer, editor, edit without view |
| **PRM-158** | new | Grant, missing | two subjects at once, delete own only, delete any only, comment rights, restriction manage without administer |
| **PRM-159** | wave F | Grant, later | Manager, manage access without administer |
| **PRM-160** | built | Restriction | none, view on root, on leaf, both, by user, group or someone else, edit on leaf, edit on parent only |
| **PRM-161** | new | Restriction, missing | view on the middle level, two subjects on one list, three levels at once, a group that no longer exists, a disabled person on the list |
| **PRM-162** | built | Page state | published, others' draft, own draft, trashed |
| **PRM-163** | wave D | Page state, later | archived, archived parent, kinds page, post, folder, link, template |
| **PRM-164** | built | Space state | active, archived, trashed |
| **PRM-165** | built | Action | view, create, edit, archive, delete, add file, delete file, manage restrictions, export, administer |
| **PRM-166** | new | Action, missing | `space.view` at space level with no page, `comment.add`, delete with a creator who differs from the page's creator |
| **PRM-167** | wave C | Action, later | `comment.delete`, comment view with `editors_only` |
| **PRM-168** | wave F | Action, later | `space.manage_access` |
| **PRM-169** | wave H | Action, later | `analytics.view` |

Today: 6 people × 11 grants × 13 restrictions × 4 page states × 3 space states × 10 actions = **102,960 cases** in `Evaluate`. The test fails if that falls below 10,000. Adding the lines marked new gives roughly 450,000, which is still quick for a pure function. The SQL half needs real rows, so it should use a smaller set (PRM-173).

## 11.3 The literal rule, for a generator

```
allowed(person, grants, chain, page, space, action) =
    person.active
    and "use_product" in person.global
    and space.status != "trashed"
    and holds(space.view)
    and page.status != "trashed"
    and (page.status != "draft" or page.created_by == person)
    and page.kind != "template"                                  # wave D
    and every level in chain.view: person matches level
    and (action in SPACE_READS or space.status != "archived")
    and (action in PAGE_READS or not page.archived)              # wave D
    and permission_for(action)
    and (action not in EDITS or chain.edit is empty or person matches chain.edit)

SPACE_READS         = view, export, administer
PAGE_READS          = view, export, delete, administer           # wave D
EDITS               = edit, archive, delete, add file, delete file
holds(p)            = some grant with p whose subject matches person
matches(user u)     = u == person
matches(group g)    = g in person.groups
matches(all_members)= person.type == member
matches(all_guests) = person.type == guest
permission_for(view)            = true
permission_for(delete)          = holds(delete_any) or (creator == person and holds(delete_own))
permission_for(delete file)     = holds(attachment.delete_any) or (file creator == person and holds(attachment.delete_own))
permission_for(manage_access)   = holds(manage_access) or holds(administer)  # wave F
permission_for(other)           = holds(other)
```

`visible` is the same without the lines that say forbidden: the permission, both archived checks, and the edit restriction. The expected status follows: allowed 2xx, visible 403, otherwise 404.

## 11.4 Tests beside the matrix

- **PRM-170** · built · **Route walk:** `TestEveryObjectRouteIsProtected` walks the real route table: a stranger gets 404, a viewer gets 403. A new route joins by itself. Routes marked authenticated are not walked, so a list on such a route needs its own test (PRM-171).
- **PRM-171** · new · **List walk:** one test builds a space with a hidden page that has one of everything: a label, a comment, a task, a file, a link, a mention, a reaction, an include. It then calls every list route as someone who may not see the page, and fails if the page's title, a planted word or its id appears in any answer. A list is registered in a table, and a test fails when a route that returns many rows is not in it.
- **PRM-172** · new · **Count walk:** the same set-up, comparing every number in an answer with what the same person gets when the hidden page does not exist. A difference means a count leaks.
- **PRM-173** · new · **A smaller product for SQL:** the SQL half builds real rows, so it runs fewer values, or all pairs of values, to stay fast. `Evaluate` keeps the full product.
- **PRM-174** · new · **Restriction survival:** for every operation that changes the place of a page (move, copy, archive, restore, import, bundle, delete and undo), a test proves that nobody can see the page who could not see it before, unless the actor held *manage restrictions*.
- **PRM-175** · built · **Mutation:** `scripts/mutation-check.sh` breaks a rule on purpose, and the tests must notice. Each new rule adds a line. A wave B mutation caught that a test proved nothing, because nobody had the searched name.
- **PRM-176** · new · **Purge walk:** after a purge and after anonymising, a test walks every table through `information_schema`. It looks for the id in every `uuid` column and for planted words in every text and JSON column. A new table is covered the moment it exists. Wave F has the text half. The id half finds rows such as `follows`.
- **PRM-177** · new · **No fall-open:** a test deletes a parent row directly, and proves no child becomes more visible (GAP-019).

## 11.5 What the rule does not test

The matrix proves the rule, not its use. A route that forgets to call a module, or a copy of text with its own audience, stays out of its sight. That is why 05.10 exists, and why the list walk matters more than more dimensions.

## 11.6 Why not row security in PostgreSQL

Considered and not chosen for now. Row security denies by default and could be a second wall. But all connections share one database role and go through a pool, so each request would have to set the person in its transaction. The documentation warns that policies which consult other tables are open to races under Read Committed, and that unique and foreign key checks bypass them. Our rule reads several tables. It could return as defence in depth in phase 3, with `FORCE ROW LEVEL SECURITY`, on the few tables where it is cheap.
