# 03.11: Import, export and publishing

Nobody should feel locked in. Everything that goes in can come out.

## 11.1 Export

- **EXC-001** · built · **Markdown:** a page as `.md`. A page with everything below it, or a whole space, as a `.zip` with folders and files.
- **EXC-002** · built · Exports contain only what the person may see, and need *export*.
- **EXC-003** · built · Every export is audited.
- **EXC-004** · phase 4 · **PDF:** one page, a branch, or a space as one file, with a table of contents.
- **EXC-005** · phase 4 · **Word (DOCX):** one page or a branch.
- **EXC-006** · phase 4 · **ODT:** the same, for office suites that prefer it.
- **EXC-007** · phase 4 · **HTML:** a zip with a small website that works offline, with an index and images.
- **EXC-008** · phase 4 · **Plain text.**
- **EXC-009** · phase 4 · **Bundle:** a signed archive of a space with history, comments, labels, permissions and files, for moving to another instance.
- **EXC-010** · new · **Choose pages:** the export dialog shows the tree with checkboxes.
- **EXC-011** · new · Options per export: include comments, include old versions, include attachments, include restricted pages the person can see.
- **EXC-012** · new · **PDF design:** a space administrator sets a cover page, header, footer with page numbers, and a logo. Paper size and orientation can be chosen when exporting.
- **EXC-013** · new · Collapsible sections are open, and dynamic blocks show their content as the exporting person sees it.
- **EXC-014** · new · A long export runs in the background. The person gets a notification with a link, valid for 24 hours.
- **EXC-015** · new · Exporting always uses the published version. Drafts are never exported.
- **EXC-016** · new · A failing page names itself in the result, and the rest continues.
- **EXC-017** · new · An administrator can turn off export formats, or limit them to certain groups.
- **EXC-018** · new · **Download all attachments** of a page (FIL-035) and of a space.
- **EXC-019** · new · A page can be printed from the browser with a print style (NAV-082).

## 11.2 Import

- **EXC-020** · built · **Markdown:** a `.md` file, or a `.zip` of folders, creates pages under a chosen parent.
- **EXC-021** · built · Imports run in the background, and report progress and problems.
- **EXC-022** · built · Limits: 200 MiB compressed, 1 GiB unpacked, 10,000 files, 20 MiB per Markdown file.
- **EXC-023** · built · Paths are never used on disk, so a hostile archive cannot escape.
- **EXC-024** · built · Images and files found in the zip become attachments. Links to other Markdown files stay links. External images become links.
- **EXC-025** · new · Links between imported Markdown files become links to the new pages.
- **EXC-026** · new · Front matter in Markdown files sets labels, status and dates.
- **EXC-027** · phase 5 · **Word (DOCX):** one file makes a page, with headings, lists, tables, images and links. Limits are set by the administrator.
- **EXC-028** · new · **Several Word files** become pages under a restricted holding page, so they can be reviewed before others see them.
- **EXC-029** · new · A Word file can also be imported into an open draft.
- **EXC-030** · phase 5 · **HTML:** a zip with HTML pages and images makes a tree. All HTML goes through the allow-list.
- **EXC-031** · phase 5 · **Confluence export:** a space export from Confluence Cloud or Data Center keeps pages, tree, attachments, comments, labels, versions and permissions where possible, from documented formats and donated samples.
- **EXC-032** · new · **Mapping users:** the importer matches authors on e-mail, and asks what to do with those it cannot match.
- **EXC-033** · new · **Dry run:** every import can first show what it would do, with a count and warnings, and change nothing.
- **EXC-034** · new · **Other wikis:** importers for common open formats, such as MediaWiki XML and exports of other Markdown wikis.
- **EXC-035** · new · An import can be undone in one action, as long as nobody edited the imported pages.
- **EXC-036** · new · Unknown content is kept as readable text with a note, never silently dropped.
- **EXC-037** · phase 4 · **Bundle:** imports the signed archive, and checks the signature.

## 11.3 Publishing to the public

ADR-010 decided that the internal instance is never reachable by the public. Public pages are a static site on a separate host, with no way back in.

- **EXC-038** · phase 5 · A space administrator publishes selected pages or a whole space to a static website.
- **EXC-039** · phase 5 · Publishing is explicit: nothing goes public by default, and a person sees exactly what will be published before they confirm.
- **EXC-040** · phase 5 · The site is built from published versions only, and contains no comments, names of editors unless chosen, or restricted pages.
- **EXC-041** · phase 5 · Updates are pushed on publish, on a schedule, or by hand.
- **EXC-042** · phase 5 · The site has its own theme, search that runs in the browser, and a sitemap.
- **EXC-043** · new · Every published page shows a mark inside the wiki, so writers know a page is public.
- **EXC-044** · new · **Single shared page:** a person with permission makes one page public as a static page with an unguessable address, which can be withdrawn. The space and the instance must both allow it. This replaces direct public links to the internal instance.
- **EXC-045** · new · Search engines can be allowed or blocked per site.
- **EXC-046** · new · Publishing is audited: who, what and when.
