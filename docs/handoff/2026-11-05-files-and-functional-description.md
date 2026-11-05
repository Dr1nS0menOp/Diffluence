# Handoff 2026-11-05: files in the text, and the functional description

## What was done

- **The narrow page.** `Frame` already wraps a screen in `.column`, and `PageView` added a second one. A flex item with auto margins shrinks to its content, so a short page was 347px wide instead of 760px. The article now uses `.page`. The pictures missed it because their text was long, so a browser test now measures a short page.
- **Files in the text (ADR-022, spec 007).** No Upload button or Attachments block on a page. Files go in by dropping them anywhere in the editor, or with the toolbar. A new `file` block in the document format shows as a tile. The list moved to More, Attachments.
- **The functional description (ADR-023).** Blueprint 03 grew from one short list to 13 chapters: 986 numbered requirements, about 52 A4, each with a status. It comes from a clean-room study of public help pages, listed in `docs/blueprint/03-functional/sources.md`. Everything is in our own words.
- `ImageResolver` and `ImageTarget` became `AttachmentResolver` and `AttachmentTarget`, because they now serve files as well.
- A new mutation proves that a `file` block must point to an attachment.

## Decisions taken along the way

- A person with *add attachments* but without *edit* can no longer add files, because adding needs the editor. This follows from the product owner's request.
- A drop beside the page lands at the cursor, or at the end if the editor had no focus.
- A tile stores the file name, not the size, so nothing goes stale when a new version arrives.
- Pasting stays limited to images, as before. A pasted office selection often carries a picture of itself, and that must not become a file.

## Unfinished, or seen along the way

- Deleting an attachment leaves its tile, and the download then fails. The study asks for a clear placeholder (chapter 6).
- Attachments uploaded earlier with the old button are not in any document, so a Markdown zip import leaves them out. They are still in the export zip.
- Comparing versions still glues list items together ("buddySet up your laptop"). Older bug, not touched.
- `ci-local` runs on a commit, and nothing is committed yet.

## What is next

The product owner decides the order: see "Proposed order" in `docs/blueprint/03-functional-requirements.md`. Each wave starts by writing specs from the chapters, with numbers in the provenance.

## For the product owner to decide

1. Should the four waves come before phase 3?
2. Wave A needs a few extra TipTap packages, from the family already in use. May they be added?
3. Chapter 13 is a list of options: which of those matter to you?
4. Commit and push this round?
