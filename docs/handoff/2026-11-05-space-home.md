# Handoff 2026-11-05: the home of a space, and an example space

## What was decided

The product owner asked for a home page that is clearly the home page, above the tree, with a strip of spaces above that, and for the first space to show everything that is possible. ADR-027 records it, and spec 016 holds the plan.

## What was done

- The sidebar has a strip of spaces, and a fixed Home row. The home page left the tree, the flat views and the filter, and its children are the top rows.
- The breadcrumbs end in Home on the home page.
- `internal/example` makes the example space through the public services, from plain files in `internal/example/content`.
- The worker makes it once, on an instance that has never had a space. `DIFFLUENCE_EXAMPLE_SPACE=false` turns that off, and `scripts/e2e` does so to keep the pictures on an empty database.
- Administration, Settings has an Add example space button, for instances that exist already.

## Found by the tests

- The chip of the current space went out of view when the second list arrived and changed the order, without changing the length. The strip now follows the place of the chip.

## For later waves

- **A new block or mark makes `TestTheExampleShowsEverythingTheFormatCanHold` fail on purpose.** Add it to a document in `internal/example/content`.
- To add a page: a document in `content/`, and a line in `content/space.json`. Placeholders are `@file:name` and `@page:slug`.
- Wave C adds comments, reactions, a mention and a task with a person. Wave D adds labels, a blog post, a page status and a template. Wave E adds dynamic blocks.

## Unfinished, or seen along the way

- If making the example fails halfway, the space stays as it is. It can be deleted and added again with the button.
- The tree has no gap after its last row, so a row cannot be dropped at the very end of the top level. That was already so.
- The Move dialog still offers the home page under its own title, and Top level beside it. Both land at the top of the tree.
