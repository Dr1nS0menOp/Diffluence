# 007: Files in the document

Status: built, waiting for the product owner's acceptance session
Phase: between 2 and 3, on the product owner's request of 2026-11-05 (ADR-022)

## Problem
A page showed an Upload button and an Attachments block under every text, also when empty. The product owner does not want that: a file belongs in the text, and is added while editing, by dragging it in.

## Behaviour

**Adding.** While editing, a person drops one or more files anywhere in the editor, on the text or beside it. The toolbar's *Add image or file* does the same for the keyboard. Safe images become images. Every other file becomes a tile. The server decides from the content. A file lands where it was dropped, or at the cursor; beside the page, at the cursor or else at the end. Several files land in order, one after the other, and a drop never replaces selected text. While dragging, the sheet shows a dashed outline. A line says what is uploading. A failed file shows why, and the others go on.

**Reading.** A tile shows an icon and the name, and downloads the file. A page has no Upload button and no Attachments block. *Attachments* in the More menu lists all files of the page, with version, size and Delete.

**The document.** A `file` block holds the attachment id and the file name. It passes the allow-list, is part of search and comparison, and follows the attachment when a page is copied. Removing a tile keeps the attachment. Deleting an attachment keeps the tile, whose download then fails with "not found".

**Exchange.** Export writes `[name](path)`. Import turns such a link into a tile when the archive holds the file. A link to another Markdown file stays a link. In a sentence, the words stay and the tile follows the sentence.

## Permissions and audit
Adding needs *add attachments*, and the tool is absent without it. A drop then says the person cannot add files here. Viewing and downloading need view on the page. Deleting follows spec 003. Events stay as in spec 003: `attachment.uploaded|downloaded|trashed|restored|purged`.

## Acceptance criteria
- [x] A short page fills the reading column
- [x] A page shows no Upload button and no Attachments block
- [x] A file dropped on the text becomes a tile, and can be downloaded after publishing
- [x] Files dropped beside the page land in the document, in order, and the browser stays on the editor
- [x] A refused upload shows the reason and changes nothing
- [x] Without *add attachments* the tool is absent and a drop explains why
- [x] Attachments in the More menu lists the file, and Delete removes it
- [x] A `file` block with an outside address, a bad id or no name is refused
- [x] A copied page points at its own copy of the file
- [x] A tile survives export and import, and a link between pages does not become a file

## How the product owner tests it
Open a page: the text fills the column and there is no button. Choose Edit and drag a PDF from the Explorer onto the text, then two files onto the empty margin. Publish. Click a tile. Open More, Attachments. Drag a very large file and read the message.

## Provenance
The product owner's request of 2026-11-05. Public help pages listed in `docs/blueprint/03-functional/sources.md` under Files, read for behaviour only. Own design.
