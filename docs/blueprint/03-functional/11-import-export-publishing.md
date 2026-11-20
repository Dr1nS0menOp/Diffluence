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
- **EXC-019** · built · A page can be printed from the browser with a print style (NAV-082).
- **EXC-049** · new · The **Export** section of space settings holds, from top to bottom: its name as the title, the choice of format, further options folded under a closed control (EXC-011), the warning of EXC-052 when the format leaves something out, the part **Content**, and one primary button at the foot that starts the export. The button is start-aligned and named for the action, and there is no Cancel, because leaving the section cancels.
- **EXC-050** · new · The format is chosen from cards as wide as the column, one card with a radio for each format that can hold a whole space and that is switched on for the person (EXC-017), with PDF first and chosen. The whole card can be pressed, and the choice shows in its radio.
- **EXC-051** · new · The part **Content** offers two plain radios, stacked: everything the person may see, which is chosen at first, and chosen content, which opens the tree of EXC-010.
- **EXC-052** · new · Before an export starts, a warning panel with a bold title, between the further options and the part Content, names the kinds of item that the chosen format cannot hold, such as databases and whiteboards in a PDF, and points to the bundle, which holds everything (EXC-009), when the person may make one. It changes when the format changes, and is absent when the format holds everything; which kinds each format can hold needs its own study (changes the design standard, decided 2026-11-14).
- **EXC-053** · new · Under the heading of the part Content, a muted explanation says that content the person may not see, or that the instance's export rules keep out (EXC-017), is left out, and which kinds of item go into the chosen format. It never counts or names what was left out (NAV-005).
- **EXC-054** · later · **CSV:** a space can be exported as a CSV list of its pages and other items with their facts, one row per item and only what the person may see. Values that a spreadsheet would read as a formula are made harmless, as in every CSV (SEC-362). (The columns, and whether this is the Content table of SPC-060 as a download, need their own study.)
- **EXC-055** · later · **Watermark:** when a watermark on images is in force for the instance or the space (chapters 6 and 10), the images in a PDF, Word, ODT, HTML or Markdown export and in Download all attachments (EXC-018) carry it, and a watermark that names the viewer names the person who exports. It is drawn into the exported copy only, never into the stored file (deliberate difference 6), and a bundle carries the stored files unchanged, because it moves the space with its settings.
- **EXC-056** · new · Beyond EXC-012, an administrator sets a **PDF design for the whole instance**, in the look group of the administration, with the same parts as the design of a space. Every space uses it until its space administrator saves a design of its own, and the space's section then offers to return to the instance's design. (Whether a space can replace single parts instead of the whole design needs its own study.)
- **EXC-057** · new · A PDF design has four parts, always in this order: the cover, the header, the footer and the page settings.
- **EXC-058** · new · A PDF design is made of fields and choices only, never of hand-written markup or style rules, so every PDF comes from our own renderer (spec 014 D1). A logo or other picture in it is an uploaded file of the space or the instance, checked as any image (FIL-019), never an outside address (deliberate difference 5).
- **EXC-059** · new · **Cover:** a page of its own before the table of contents, with the logo, the title of what is exported, a subtitle and the date of the export. A design whose cover is empty gives a PDF without one. (What else a cover may hold needs its own study.)
- **EXC-060** · new · **Header and footer:** each has three places along the edge of the page, at the start, in the centre and at the end. Each place holds a short text with the fields `{page}`, `{pages}`, `{title}` and `{date}`, or the logo, so a page number can show alone or with the number of pages; they show on every page except the cover.
- **EXC-061** · new · **Page settings, the page:** the default paper size, chosen from the sizes the export dialog offers or given as a width and height of its own in millimetres, the orientation, and the four margins in millimetres. The size and orientation chosen when exporting still win (EXC-012).
- **EXC-062** · new · **Page settings, the text:** the font, chosen from the system font, the instance's own font (ADM-015) and the PDF font (EXC-069); the size and colour of body text, which lists and tables follow; and the size and colour of header and footer text, muted at first. Colours are chosen by name (deliberate difference 10).
- **EXC-063** · new · A new instance starts with a design that has an empty cover, an empty header, and the page number with the number of pages in the centre of the footer, on A4 in portrait (decided on the product owner's behalf on 2026-11-14, may be overruled).
- **EXC-064** · new · The **PDF design** section, in the look group of space settings and of the administration, shows from top to bottom: its name as the title, one line of explanation, then the four parts stacked under each other, each under a small bold label set tight above it, with about one line of space between parts.
- **EXC-065** · new · A part that is empty shows as one quiet bar across the column, with a muted sentence that invites the person to start it; pressing the bar, or Enter on it, opens the part's fields in its place. A filled part shows its fields.
- **EXC-066** · new · In a space, the line of explanation ends with a link that exports the whole space as PDF with the design as saved, so the design can be tried at once.
- **EXC-067** · new · Save, the primary button, and Reset changes sit under the four parts at the start edge, as NAV-162 says.
- **EXC-068** · new · A PDF shows text in every script that the instance's languages use, including right-to-left scripts and those of East Asian languages, with no empty boxes for missing letters (NAV-075). (Which scripts the fonts of the PDF helper cover, and their licences, needs its own study.)
- **EXC-069** · later · **PDF font:** a section in the Instance group of the administration (ADM-071) lets an administrator add one extra font for PDF export, for a script the shipped fonts lack. Exports then use it for every letter the shipped fonts lack, with no change to any design; a new upload replaces it, and removing it returns to the shipped fonts.
- **EXC-070** · later · The PDF font is accepted only from an administrator, by its first bytes as TrueType or a TrueType collection, within a size limit of its own, because the 2 MiB of SEC-336 is too small for fonts of East Asian scripts. It travels to the PDF helper with each export, so the helper never fetches anything (ARC-085), and adding or removing it is audited.
- **EXC-071** · later · The **PDF font** section shows from top to bottom: its name as the title; a status line that names the font in use in bold (the uploaded file, or the shipped fonts), followed by a small action that removes the uploaded font when there is one; a short explanation; a file field with Install as the primary button under it; and a note that only one extra font is kept and that a new upload replaces it.

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
- **EXC-047** · phase 11 · **Leaving for Confluence and other apps:** a space can be moved from Kenwerk to Confluence and other apps, with as much as those formats can hold, and a report of what could not come along.
- **EXC-048** · phase 11 · **Other large tools:** people can migrate to Kenwerk from other large tools. Which ones follows from what people ask for by then.
- **EXC-035** · built · An import can be undone in one action, as long as nobody edited the imported pages.
- **EXC-036** · built · Unknown content is kept as readable text with a note, never silently dropped.
- **EXC-037** · built · **Bundle:** imports the signed archive, and checks the signature.
- **EXC-072** · new · Beyond EXC-037, the administration has an **Import a space** section where a bundle comes in. From top to bottom: its name as the title, a short part saying that a space from another instance comes in this way, an information panel with a bold title, the form under a sub-heading of its own, and one primary Import button under the form, start-aligned.
- **EXC-073** · new · The information panel, shown before a file is chosen, says what an import does: it makes one new space with its pages, history, comments, labels, permissions and files. It also says that a bundle never carries passwords, sessions, tokens, audit events or view data (spec 014 D12).
- **EXC-074** · new · The form holds one required file field, marked as required, with a hint under it that only the bundle of one space is accepted, not a backup of a whole instance. Import stays inactive until a file is chosen, with a line beside it that says the file is missing (ADM-094).
- **EXC-075** · new · A file that is not the bundle of a space, such as a backup of a whole instance, is refused with a sentence that says what it seems to be and where a backup is restored (OPS-118).
- **EXC-076** · new · While bundles are switched off for the installation (ADR-043), the section shows one line that says so in place of the panel and the form.
- **EXC-077** · later · When a connector to a work tracker is set up (roadmap, "Connectors for work and code tools"), the form also offers, under the file field, an optional project of that tracker to link the new space to, with a hint under it: none at first, and only projects the connector may see. (Whether this belongs in the import or only in the space's settings afterwards needs its own study.)
- **EXC-078** · later · Beyond EXC-033, before a large import from another product (phase 11), an **assessment** reads an uploaded export file without importing anything, and reports the spaces, pages, people and files it holds. It reads an uploaded file only, never a live connection to the other product, and sends nothing out (SEC-900).
- **EXC-079** · later · The assessment lists every kind of content that will not come along whole, such as dynamic blocks, automations and add-on content without an equal here, grouped by how much work it needs, each with how it will arrive (as it is, as text, or not at all) and a suggested way to carry it over. It also suggests which spaces to move first and which to clean up before moving.

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
- **EXC-080** · new · Beyond EXC-044, the administration has a **Single shared pages** section. From top to bottom: its name as the title; two short paragraphs saying that a single shared page is a static copy on the public host that anyone with its address can read, and that the administrator decides which spaces may share; a help link; and a switch, with its label after it, that allows single shared pages on the instance and is off at first.
- **EXC-081** · new · Once the switch is on, the section says where single shared pages are allowed: in every space, or only in chosen spaces, shown as chips. A space that is not allowed cannot turn them on (EXC-086).
- **EXC-082** · new · Under that, the section lists every single shared page of the instance with its title, its space, who made it and when, newest first, with filters by space and by person above the list. A box on each row and one action for the chosen rows withdraw one or several at once, after a confirmation; the list reveals nothing, because each page in it is public already (deliberate difference 2).
- **EXC-083** · new · Turning the instance's switch off asks first, says how many single shared pages that withdraws, and withdraws them all (decided on the product owner's behalf on 2026-11-14, may be overruled).
- **EXC-084** · new · The defaults for new spaces in the administration (ADM-023) have a part for single shared pages, after the people who get access by default: a heading, a two-line explanation that a single shared page lets people outside read one page without an account, with a help link, and a switch with its label after it, off at first. It sets only the starting value of a new space; its space administrator can change it later, and the instance must still allow single shared pages (EXC-080).
- **EXC-085** · new · Space settings have a **Single shared pages** section for a person with *administer* on the space (spec 014 D20). From top to bottom: its name as the title, the note of EXC-086 when the instance does not allow them here, two short lines on what a single shared page is with a help link, and, when they are allowed, the switch and the list of EXC-087.
- **EXC-086** · new · When the instance does not allow single shared pages in the space, the section shows an information note that says so, and nothing to change (NAV-004). An administrator of the instance also sees a link at the end of the note to the setting that allows them (EXC-081).
- **EXC-087** · new · When they are allowed, a switch turns single shared pages on or off for the space, and a list shows the space's shared pages with who made each and when, newest first, with Withdraw on each row. The list holds only pages the person may see (NAV-006).
- **EXC-088** · new · Turning them off for the space asks first, says how many shared pages that withdraws, and withdraws them. (Whether it should rather stop only new ones is the product owner's choice.)
- **EXC-089** · new · The two lines of explanation say, with an example of use, that a single shared page is a static copy on the public host that anyone with its address can read, with no way back into the instance (ADR-010).
