# 03.11: Import, export and publishing

Nobody should feel locked in. Everything that goes in can come out.

## 11.1 Export

- **EXC-001** · built · **Markdown:** a page as `.md`. A page with everything below it, or a whole space, as a `.zip` with folders and files.
- **EXC-002** · built · Exports contain only what the person may see, and need *export*.
- **EXC-003** · built · Every export is audited.
- **EXC-004** · built · **PDF:** one page, a branch, or a space as one file, with a table of contents.
- **EXC-005** · built · **Word (DOCX):** one page or a branch.
- **EXC-006** · built · **ODT:** the same, for office suites that prefer it.
- **EXC-007** · built · **HTML:** a zip with a small website that works offline, with an index and images.
- **EXC-008** · built · **Plain text.**
- **EXC-009** · built · **Bundle:** a signed archive of a space with history, comments, labels, permissions and files, for moving to another instance.
- **EXC-010** · built · **Choose pages:** the export dialog shows the tree with checkboxes.
- **EXC-011** · built · Options per export: include comments, include old versions, include attachments, include restricted pages the person can see.
- **EXC-012** · partly · **PDF design:** a space administrator sets a cover page, header, footer with page numbers, and a logo. Paper size and orientation can be chosen when exporting.
- **EXC-013** · built · Collapsible sections are open, and dynamic blocks show their content as the exporting person sees it.
- **EXC-014** · built · A long export runs in the background. The person gets a notification with a link, valid for 24 hours.
- **EXC-015** · built · Exporting always uses the published version. Drafts are never exported.
- **EXC-016** · built · A failing page names itself in the result, and the rest continues.
- **EXC-017** · built · An administrator can turn off export formats, or limit them to certain groups.
- **EXC-018** · built · **Download all attachments** of a page (FIL-035) and of a space.
- **EXC-019** · new · A page can be printed from the browser with a print style (NAV-082).

## 11.2 Import

- **EXC-020** · built · **Markdown:** a `.md` file, or a `.zip` of folders, creates pages under a chosen parent.
- **EXC-021** · built · Imports run in the background, and report progress and problems.
- **EXC-022** · built · Limits: 200 MiB compressed, 1 GiB unpacked, 10,000 files, 20 MiB per Markdown file.
- **EXC-023** · built · Paths are never used on disk, so a hostile archive cannot escape.
- **EXC-024** · built · Images and files found in the zip become attachments. Links to other Markdown files stay links. External images become links.
- **EXC-025** · built · Links between imported Markdown files become links to the new pages.
- **EXC-026** · built · Front matter in Markdown files sets labels, status and dates.
- **EXC-027** · built · **Word (DOCX):** one file makes a page, with headings, lists, tables, images and links. Limits are set by the administrator.
- **EXC-028** · built · **Several Word files** become pages under a restricted holding page, so they can be reviewed before others see them.
- **EXC-029** · built · A Word file can also be imported into an open draft.
- **EXC-030** · built · **HTML:** a zip with HTML pages and images makes a tree. All HTML goes through the allow-list.
- **EXC-031** · phase 11 · **Confluence export:** a space export from Confluence Cloud or Data Center keeps pages, tree, attachments, comments, labels, versions and permissions where possible, from documented formats and donated samples.
- **EXC-032** · phase 11 · **Mapping users:** the importer matches authors on e-mail, and asks what to do with those it cannot match.
- **EXC-033** · built · **Dry run:** every import can first show what it would do, with a count and warnings, and change nothing.
- **EXC-034** · phase 11 · **Other wikis:** importers for common open formats, such as MediaWiki XML and exports of other Markdown wikis.
- **EXC-047** · phase 11 · **Leaving for Confluence and other apps:** a space can be moved from Diffluence to Confluence and other apps, with as much as those formats can hold, and a report of what could not come along.
- **EXC-048** · phase 11 · **Other large tools:** people can migrate to Diffluence from other large tools. Which ones follows from what people ask for by then.
- **EXC-035** · built · An import can be undone in one action, as long as nobody edited the imported pages.
- **EXC-036** · built · Unknown content is kept as readable text with a note, never silently dropped.
- **EXC-037** · built · **Bundle:** imports the signed archive, and checks the signature.

## 11.3 Publishing to the public

ADR-010 decided that the internal instance is never reachable by the public. Public pages are a static site on a separate host, with no way back in.

- **EXC-038** · built · A space administrator publishes selected pages or a whole space to a static website.
- **EXC-039** · built · Publishing is explicit: nothing goes public by default, and a person sees exactly what will be published before they confirm.
- **EXC-040** · built · The site is built from published versions only, and contains no comments, names of editors unless chosen, or restricted pages.
- **EXC-041** · built · Updates are pushed on publish, on a schedule, or by hand.
- **EXC-042** · built · The site has its own theme, search that runs in the browser, and a sitemap.
- **EXC-043** · built · Every published page shows a mark inside the wiki, so writers know a page is public.
- **EXC-044** · built · **Single shared page:** a person with permission makes one page public as a static page with an unguessable address, which can be withdrawn. The space and the instance must both allow it. This replaces direct public links to the internal instance.
- **EXC-045** · built · Search engines can be allowed or blocked per site.
- **EXC-046** · built · Publishing is audited: who, what and when.
