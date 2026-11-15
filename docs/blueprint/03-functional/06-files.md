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
- **FIL-011** · built · The upload shows progress as a percentage for large files, and can be cancelled.
- **FIL-012** · built · A file with the same name as one already on the page asks: add as a new version, or keep both under another name.
- **FIL-013** · built · Dragging a file onto an existing tile replaces it with a new version.
- **FIL-014** · built · Dropping an archive does not unpack it.
- **FIL-015** · built · A picker lists files already on the page, so one file can appear more than once without a new upload.
- **FIL-016** · built · A file is stored on the page at once, also before publishing. If the person discards the draft, the attachment stays in the list and can be deleted there.
- **FIL-017** · built · Attachments that no version of the page uses, and that came from a discarded draft, are offered for clearing up.

## 6.2 Limits and safety

- **FIL-018** · built · The maximum size is set by an administrator (default 100 MiB). A larger upload is refused and leaves nothing behind.
- **FIL-019** · built · Files are never trusted. Only PNG, JPEG, GIF and WebP, detected from their content, show in the browser. Everything else, including SVG and HTML, is sent as a download with a locked-down policy.
- **FIL-020** · built · An administrator can block file types by extension and content, and can set a limit per space.
- **FIL-021** · later · Files can go through a virus scanner, through a standard interface such as ICAP. A file waits in quarantine until it has passed.
- **FIL-022** · built · Access always follows the page. Someone who may not see the page gets "not found".
- **FIL-023** · built · File names are shown, never used as paths, and are cleaned of control characters.
- **FIL-052** · later · An administrator sets how many files one drop, or one choice in the file picker, may add at once, in a number field under the largest upload size (ADM-020). Empty means no limit, as today. A larger drop says so before anything uploads, and adds nothing.
- **FIL-053** · later · An administrator can turn off downloading files for the instance or for chosen spaces, or allow it only to chosen groups. Images still show in pages and previews still open where one exists (FIL-027), while the download buttons and Download all (FIL-035) are absent for the others.
- **FIL-054** · later · The download setting of FIL-053 says that it limits copying but cannot prevent it, because a reader can still take a picture of the screen. Every download that remains allowed is audited (SEC-329).

## 6.3 Reading a file

- **FIL-024** · built · A file shows as a tile with an icon and its name. Selecting it downloads the file.
- **FIL-025** · built · The tile also shows the type and size.
- **FIL-026** · partly · A tile can be small (one line) or large (with a preview image of the first page). Still open: a preview image of the first page, which needs a renderer (spec 024).
- **FIL-027** · built · A preview opens on top of the page for images, PDF, text and common office formats, with download and next and previous. Previews never run code from the file.
- **FIL-028** · built · Text files show with syntax highlighting in the preview.
- **FIL-029** · built · A tile whose attachment was deleted says so, and is not a broken link.
- **FIL-030** · built · The name of a file is part of the document, so search finds it and comparing versions shows when it came or went.

## 6.4 The list of files

- **FIL-031** · built · **Attachments** in the More menu of a page lists all its files, with thumbnail, name, version and size.
- **FIL-032** · built · Anyone who can view the page can see the list and download. The empty list explains how to add a file.
- **FIL-033** · built · A person with the right to delete removes a file from the list. It goes to the trash. Removing a tile from the text does not delete the file.
- **FIL-034** · built · The list can be sorted by name, date, size and type, and filtered by name.
- **FIL-035** · built · **Download all** gives one zip of all files of the page.
- **FIL-036** · built · Each row shows who added it and when, and whether the current page uses it.
- **FIL-037** · built · **Versions:** a row opens to show every version, with date, person and size. Old versions can be downloaded. A person with *delete any* can remove an old version.
- **FIL-038** · built · An older version can be made the current one, which adds a new version.
- **FIL-039** · built · **File details:** rename, add a comment, add labels, move to another page. Moving needs *add* on the target and *delete* on the source, and leaves tiles pointing at the moved file working.
- **FIL-040** · built · A file can be copied to another page.
- **FIL-041** · built · Attachments can have labels, and search can filter on them.
- **FIL-042** · built · A direct link to a file can point to the latest version or to one version.

## 6.5 Files of a whole space

- **FIL-043** · built · A **Files** screen per space lists all files in that space, with page, size, date and uploader, filtered by name, type, label and person. It respects restrictions.
- **FIL-044** · built · The screen shows how much storage a space uses, and which files are largest.
- **FIL-045** · built · Space administrators can delete files in bulk from it.
- **FIL-055** · new · Beyond FIL-043, the Files screen of a space shows for each file, in this order: an icon of its kind with its name, its size, who added it, when it was added, when it last changed, its labels, and the item it belongs to with that item's type icon. The name, the person and the item are links, and a file name that occurs on two items is listed once for each.
- **FIL-056** · new · A person who may change a file edits its labels in its row of the Files screen. A file without labels says so in muted words, with an edit button after them.
- **FIL-057** · new · The Files screen sorts on each column, with the sort column and its direction marked on its header. By default the last change comes first.
- **FIL-058** · new · Under the heading of the Files screen the filters come first, with no explanatory line: one row at the start side above the table, with name, kind, file extension, label and person. They apply as the person types, as other filters do, so the row needs no button, and they offer only labels and people from items the person may see (spec 024).

## 6.6 Pages and files together

- **FIL-046** · built · Copying a page copies its files, and the copy's tiles and images point at the copies.
- **FIL-047** · built · Moving a page takes its files along.
- **FIL-048** · built · Exports contain the files. A Markdown link to a file in the export becomes a tile again on import, and a link to another Markdown file stays a link.
- **FIL-049** · built · Purging a page removes its files from storage.
- **FIL-050** · built · The page history shows when files were added or removed.
- **FIL-051** · later · A person opens an office file from the tile in their desktop program and saves back to a new version, through WebDAV.

## 6.7 Watermarks

A watermark marks the images of a space as they are shown, to discourage passing them on. It is a deterrent, not a protection.

- **FIL-059** · later · A space administrator can put a **watermark** on the images of the space, in a Watermark section of space settings. The kind is a choice of two tiles, each a title with a muted line under it: an uploaded picture, chosen by default, or the name of the person viewing.
- **FIL-060** · later · The picture for a watermark is added in an upload zone, by a link that opens the file chooser or by dropping a file on it, and is checked like any image (FIL-019).
- **FIL-061** · later · Under the kind, the form sets where the watermark sits on the image, as a choice of places with the centre by default, and how see-through and how large it is, each as a slider with a number field in percent, both 50 by default.
- **FIL-062** · later · A status row across the top of the section shows an icon, the title and whether a watermark is in force, and, while the part is experimental (ADR-043), a mark that says so.
- **FIL-063** · later · Beside the form, at its end side, a preview with its own heading shows the watermark on an image, with a choice of what it is drawn on and a grip that resizes the preview. On a narrow screen the preview moves below the form.
- **FIL-064** · later · Save and Reset changes end the watermark form, as NAV-162 says.
- **FIL-065** · later · The stored file never changes: the watermark is drawn wherever an image of the space appears, when it is shown, downloaded or exported (EXC-055), and only on the four safe image types (FIL-019). A watermark with the viewer's name shows only to that viewer and is never stored.
- **FIL-066** · later · A watermark with the viewer's name offers a choice of light or dark text, so the name stays readable on light and dark images.
- **FIL-067** · later · A space's watermark takes the place of the instance's watermark (chapter 10) for the images of that space.
- **FIL-068** · later · An animated image, and an image larger than 100 MiB, is shown without a watermark, and the Watermark section says so.
- **FIL-069** · later · A space administrator removes the watermark in one action. Its settings stay saved, so applying it again brings it back as it was.
