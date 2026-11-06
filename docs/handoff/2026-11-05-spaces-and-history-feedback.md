# Handoff 2026-11-05: animals, changes in the page, people in pickers

## What was decided

- ADR-031: spaces have animals, and a comparison is the page itself. Spec 018 holds the study and the plan.

## What was done

- 30 cartoon animals from Kenney's Animal Pack Remastered (CC0), in `web/public/animals/`. A space gets one from its key, with no migration, and can pick another or a character in Space settings.
- `Compare` also sends the blocks themselves, and `DiffDocument` shows them as the page with marks on top.
- Opening a version in History shows its changes, with a button for the clean page.
- `document.ChangesText` and `document.ChangesHTML` turn a comparison into text and safe HTML for mail.
- `SubjectPicker` is a combobox that shows people on focus, and works by keyboard.

## Unfinished, or seen along the way

- The mail about a new version of a watched page now shows the changes, through `document.Compare`, `ChangesText` and `ChangesHTML`, in `internal/notify/render.go`. It compares when the mail is sent, after access was checked again, and only when the instance allows content in mail (COL-076). The words Added and Removed sit with the other mail words, until mail follows a person's language.
- A change that only touches formatting, such as bold or a heading level, does not show: blocks compare by text.
- Comparing tables per cell (PAG-092) is still open: a changed table shows as it was and as it is.
- A status mark or a date in a paragraph makes that block fall back to removed and added.
- `--update-snapshots` leaves a picture alone when the change is under the tolerance. Use `--update-snapshots=all` after a small change of look that shows on many screens.

## What is next

Nothing is waiting on this work.
