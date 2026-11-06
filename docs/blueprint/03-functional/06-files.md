# 03.6: Files

Decided by the product owner on 2026-11-05 (ADR-022): **a file belongs in the text**. A page has no upload button. A person adds files while editing, by dragging them in, and readers meet a file at the place where the writer put it.

## 6.1 Adding a file

- **FIL-001** · built · While editing, a person drops one or more files anywhere in the editor: on the text, on the title, or in the margin beside the sheet.
- **FIL-002** · built · A dropped file lands where it was let go. In the margin it lands at the cursor, or at the end if the editor had no focus.
- **FIL-003** · built · While dragging over the editor, the sheet shows a dashed outline, so it is clear that a drop will work.
- **FIL-004** · built · Several files land in the order they were dropped.
- **FIL-005** · built · A drop never replaces selected text.
- **FIL-006** · built · The **Add image or file** tool does the same, for the keyboard and for people who prefer a dialog. It allows several files at once.
- **FIL-007** · built · Images that the browser may show become images. Everything else becomes a file tile. The server decides this from the content, not from the name.
- **FIL-008** · built · While a file uploads, a line says which one.
- **FIL-009** · built · A failed upload shows the reason, such as the size limit, and leaves the document as it was. Other files in the same drop continue.
- **FIL-010** · built · A person without *add attachments* sees no tool, and a drop says that they cannot add files here.
- **FIL-011** · new · The upload shows progress as a percentage for large files, and can be cancelled.
- **FIL-012** · built · A file with the same name as one already on the page asks: add as a new version, or keep both under another name.
- **FIL-013** · new · Dragging a file onto an existing tile replaces it with a new version.
- **FIL-014** · new · Dropping an archive does not unpack it.
- **FIL-015** · new · A picker lists files already on the page, so one file can appear more than once without a new upload.
- **FIL-016** · built · A file is stored on the page at once, also before publishing. If the person discards the draft, the attachment stays in the list and can be deleted there.
- **FIL-017** · new · Attachments that no version of the page uses, and that came from a discarded draft, are offered for clearing up.

## 6.2 Limits and safety

- **FIL-018** · built · The maximum size is set by an administrator (default 100 MiB). A larger upload is refused and leaves nothing behind.
- **FIL-019** · built · Files are never trusted. Only PNG, JPEG, GIF and WebP, detected from their content, show in the browser. Everything else, including SVG and HTML, is sent as a download with a locked-down policy.
- **FIL-020** · new · An administrator can block file types by extension and content, and can set a limit per space.
- **FIL-021** · later · Files can go through a virus scanner, through a standard interface such as ICAP. A file waits in quarantine until it has passed.
- **FIL-022** · built · Access always follows the page. Someone who may not see the page gets "not found".
- **FIL-023** · built · File names are shown, never used as paths, and are cleaned of control characters.

## 6.3 Reading a file

- **FIL-024** · built · A file shows as a tile with an icon and its name. Selecting it downloads the file.
- **FIL-025** · new · The tile also shows the type and size.
- **FIL-026** · new · A tile can be small (one line) or large (with a preview image of the first page).
- **FIL-027** · partly · A preview opens on top of the page for images, PDF, text and common office formats, with download and next and previous. Previews never run code from the file. Still open: images, PDF and office files, and opening from a tile. Text shows under Details.
- **FIL-028** · new · Text files show with syntax highlighting in the preview.
- **FIL-029** · new · A tile whose attachment was deleted says so, and is not a broken link.
- **FIL-030** · built · The name of a file is part of the document, so search finds it and comparing versions shows when it came or went.

## 6.4 The list of files

- **FIL-031** · built · **Attachments** in the More menu of a page lists all its files, with thumbnail, name, version and size.
- **FIL-032** · built · Anyone who can view the page can see the list and download. The empty list explains how to add a file.
- **FIL-033** · built · A person with the right to delete removes a file from the list. It goes to the trash. Removing a tile from the text does not delete the file.
- **FIL-034** · new · The list can be sorted by name, date, size and type, and filtered by name.
- **FIL-035** · built · **Download all** gives one zip of all files of the page.
- **FIL-036** · new · Each row shows who added it and when, and whether the current page uses it.
- **FIL-037** · built · **Versions:** a row opens to show every version, with date, person and size. Old versions can be downloaded. A person with *delete any* can remove an old version.
- **FIL-038** · built · An older version can be made the current one, which adds a new version.
- **FIL-039** · partly · **File details:** rename, add a comment, add labels, move to another page. Moving needs *add* on the target and *delete* on the source, and leaves tiles pointing at the moved file working. Still open: labels and moving to another page.
- **FIL-040** · new · A file can be copied to another page.
- **FIL-041** · new · Attachments can have labels, and search can filter on them.
- **FIL-042** · new · A direct link to a file can point to the latest version or to one version.

## 6.5 Files of a whole space

- **FIL-043** · partly · A **Files** screen per space lists all files in that space, with page, size, date and uploader, filtered by name, type, label and person. It respects restrictions. Still open: filters other than the name.
- **FIL-044** · built · The screen shows how much storage a space uses, and which files are largest.
- **FIL-045** · new · Space administrators can delete files in bulk from it.

## 6.6 Pages and files together

- **FIL-046** · built · Copying a page copies its files, and the copy's tiles and images point at the copies.
- **FIL-047** · built · Moving a page takes its files along.
- **FIL-048** · built · Exports contain the files. A Markdown link to a file in the export becomes a tile again on import, and a link to another Markdown file stays a link.
- **FIL-049** · built · Purging a page removes its files from storage.
- **FIL-050** · new · The page history shows when files were added or removed.
- **FIL-051** · later · A person opens an office file from the tile in their desktop program and saves back to a new version, through WebDAV.
