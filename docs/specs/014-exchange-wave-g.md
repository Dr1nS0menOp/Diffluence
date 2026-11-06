# 014: Wave G, everything can leave and come in

Status: built on branch `wave-g`, except what the handoff note lists
Phase: wave G of `docs/blueprint/03-functional-requirements.md`, covering chapter 11. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave. It comes after waves C to F, because a bundle carries their comments, labels and files.

## Problem

Today a space can leave as Markdown and come in as Markdown, and that is all. Nobody can hand a colleague a PDF, move a space to another instance with its history, or bring in what they already have in Word, HTML or another wiki. There is no way to show a page to the outside world either. Chapter 11 promises that nobody feels locked in. This spec is the plan for keeping that promise, without ever opening the instance to the public.

## What the study found

**What people expect from export.** Every wiki studied exports one page, a branch or a space. The dialog shows the tree with checkboxes. PDF gets a cover, a header and a footer. Long exports run in the background and end with a link. Exporting uses what is published, never a draft.

**What people expect from import.** They first want to see what will happen. Authors should be matched where possible, and they want one action to take a bad import back. Losing content silently is what they fear most, so anything unknown has to stay visible.

**The formats.**

| Format | What is public | What it means for us |
|---|---|---|
| DOCX | A zip package with a content types file and relationships, and a main part with `w:document`, `w:body`, `w:p`, `w:r`, `w:t`. ISO/IEC 29500 parts 1, 2 and 4. The name of the main part must be found through the relationships, not assumed. | Our blocks are a short fixed list, so writing that subset is small. Reading follows relationships. |
| ODT | A zip package with `mimetype`, `content.xml`, `styles.xml`, `meta.xml`, and a manifest. ISO/IEC 26300, OASIS 1.2. | The same shape as DOCX, with other element names. |
| PDF | Tagged PDF is what makes it readable to assistive technology. PDF/UA is ISO 14289. Chromium has written tagged PDF since version 85, and that alone does not promise PDF/UA. | Writing PDF by hand would lose tags, font shaping and right-to-left. A browser engine does this well, so PDF needs a helper process. |
| Confluence space export | A zip with `entities.xml`, `exportDescriptor.properties` and an attachments folder. `entities.xml` has a `hibernate-generic` root with `object` elements that hold `id`, `property`, `collection` and `element`. Order in collections cannot be relied on, and relations must be read from both sides. Older and newer properties for users both occur. A public issue reports exports that hold more than one space object. | Stream it, index it, and treat every link as a hint from two sides. |
| Confluence storage format | Described by its maker as XHTML-based, and technically XML with custom elements: `ac:link` with `ri:page`, `ac:image` with `ri:attachment` or `ri:url`, `ac:link-body`, and macros. | `encoding/xml` reads it. It needs no HTML parser. |
| MediaWiki XML | A `mediawiki` root with `page`, `revision`, `timestamp`, `contributor`, `comment`, `text`. `text` holds wikitext, not rendered content. Files are not part of it. | A small wikitext reader for the common subset, and the rest stays as text. |
| Sitemap | `urlset`, `url`, `loc`. 50,000 addresses or 50 MB per file, and an index file above that. A sitemap only lists addresses at or below its own place. | One file, or an index for very large sites. |

**What accessibility demands.**

| Source | Demand | What we do |
|---|---|---|
| 1.3.1 Info and relationships | Structure must survive in what we make. | DOCX and ODT use real heading styles, table header rows and alt text. HTML has landmarks and `lang`. PDF comes from that same HTML, tagged. |
| Tree view pattern | A tree item may not hold other controls. | Choose pages is a nested list of native checkboxes in a `fieldset`, not a `tree`. A parent uses the native `indeterminate` state. |
| 4.1.3 Status messages | Progress reaches screen readers without moving focus. | `role="status"` beside a `progress` element, with few updates. |
| 2.2.1 Timing adjustable | A time limit needs a way out, unless it is longer than 20 hours. | A download link of 24 hours falls under that exception, and the file can be made again. |
| 3.3.4 Error prevention | An important action can be checked first, and undone. | Dry run before importing, a preview before publishing, Undo for an import, and Withdraw for a site. |
| 1.1.1 Non-text content | Images keep their text alternative in every format. | `alt` travels into DOCX, ODT, HTML and PDF. A missing one warns in the publish preview. |
| 2.4.1 Bypass blocks, 3.1.1 Language | A public site is a website of its own. | Skip link, landmarks and `lang` on every page. axe runs on the built site. |
| 2.1.1 Keyboard | Site search works without a mouse, and the site works without script. | A plain form, a live region for results, and an index page of all pages that needs no script. |

**What the libraries allow.** Go's `archive/zip` and `encoding/xml` cover the packages and the XML. `encoding/xml` never fetches external entities and has no entity expansion of its own, which removes the usual XML attacks, as long as `Decoder.Entity` stays small. It can stream by token, so a huge `entities.xml` does not need the memory. `image/jpeg` and `image/png` can re-encode an image, which drops its metadata. `internal/keys` already derives an Ed25519 signing key. There is no widely used Go module under our licences that reads and writes DOCX well, and `golang.org/x/net/html` is not in `go.mod` yet.

**Safety.** Every file is hostile: zip bombs, paths out of the archive, deep nesting, relationships that point outside (`TargetMode="External"`), and HTML with scripts. Published pages can carry more than their text: an image can hold a location and a name in its metadata, and a page can link to something secret. Each export is a copy of data that then lives outside all our rules, so it needs the same permission check as the screen, and an end date.

**Clean room.** Formats may be read from public documentation. Sample exports may only be donated by third parties from their own data. No Atlassian product or account is used, and neither is the product owner's employer's Confluence or its exports. The XWiki project's public documentation of the format was read as documentation. Its converter's source code was not read, and must not be. The words "Import from Confluence" are allowed as a description, with no logo.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | One renderer, `document.ToHTML`, built on the allow-list and escaping everything | HTML export, the public site and PDF share it. It leaves the instance only as a file. PAG-122 stays true: the instance itself never serves document HTML to a browser. |
| D2 | Own writers and readers for the DOCX and ODT subset, with `archive/zip` and `encoding/xml` | The block list is short and no suitable module exists. Output is not security critical, and reading is limited and fuzzed. Blueprint 04 named Pandoc for this. Dropping it saves a sidecar, and this needs a line in the ADR log. |
| D3 | PDF comes from an optional helper process. Without one, the PDF choice is absent and browser print remains | See "For the product owner to decide". |
| D4 | Every export except a single Markdown page runs as a job, and its file lives 24 hours | Same path for small and large, and copies do not pile up. |
| D5 | Only the person who started an export can download it, and the check runs again at download | A link in a notification must not become a way around a restriction. |
| D6 | Pages are checked one by one while building, through the normal rules | A page restricted a second ago is left out, and the result says that something was skipped without naming it. |
| D7 | "Include restricted pages I can see" starts off | A file travels. A person should choose this. |
| D8 | All importers build the same `Plan`, and one writer carries it out | Dry run, user mapping, limits and Undo then exist once. |
| D9 | Unknown content becomes a Note panel with its readable text, and a warning in the report | EXC-036: never silently dropped. |
| D10 | Undo of an import moves the pages to the trash, in one action, while nobody else touched them | It stays recoverable, and it never removes someone's work. |
| D11 | The bundle is our own zip with a `manifest.json` holding a SHA-256 per file, and a detached Ed25519 signature | Standard library only. Ids and `origin_instance_id` stay, which phase 8 needs. |
| D12 | A bundle holds no passwords, sessions, tokens, audit events or view data | Secrets stay home, and the audit trail belongs to the instance that made it. |
| D13 | A wrong signature always stops an import. An unknown signer stops it until an administrator enters the fingerprint | Tampering has no override. Trust is a choice with a trace. |
| D14 | Public sites are built by the worker into a folder or a zip. The instance never serves them | ADR-010. The host is someone else's server. |
| D15 | A site only holds pages picked by hand, with no view restriction on or above them, in their published version | Nothing is public by default, and no person's rights decide what the world sees. |
| D16 | Only files a published page shows go along. JPEG and PNG are re-encoded without metadata | A page's other attachments stay private, and photos do not leak where they were taken. GIF and WebP cannot be cleaned by the standard library, so the preview warns about them. |
| D17 | A scrub step reads the built site and stops the build on any instance address, `/api/` path, internal page id or e-mail address of a user | The last line of defence fails closed. |
| D18 | A page that gets a view restriction, or is deleted, leaves the site in a build that starts at once | A restriction must not wait for the next schedule. |
| D19 | Site search is a small JSON index and about a hundred lines of our own script | No library, and no search server on the host. |
| D20 | Public links and sites need *administer* on the space, and a switch for the instance | Chapter 11 says "a person with permission". A new permission can follow if real use asks for it. |
| D21 | Front matter is a small subset: `key: value` and simple lists. Anything else gives a warning | Not critical, so no YAML dependency. |

## The product owner's decisions (ADR-026)

PDF through Gotenberg as an optional container. `golang.org/x/net/html` joins. Public sites are a folder and a zip.

**Moved out of this wave (ADR-028):** The product owner called migration a very large job that comes very late. The Confluence importer (G12, EXC-031), mapping users for it (EXC-032), other wikis (EXC-034), and everything else in this plan about Confluence wait for phase 11. Ignore those parts here. What was studied stays below as the start of that phase's study.

## For the product owner to decide

| # | Question | Options | Advice |
|---|---|---|---|
| 1 | How is PDF made? | **a.** Gotenberg as an optional container (MIT). It holds headless Chromium and LibreOffice, is called over HTTP, and needs no Go module. It offers PDF/UA, and could later serve file previews (FIL-027). It is a large image. **b.** A small container of our own with headless Chromium, as blueprint 04 says, and a thin wrapper to maintain. **c.** No PDF on the server: browser print only, and EXC-004 and EXC-012 stay open. | **a.** It runs with no network, and it only gets HTML that we made. |
| 2 | May `golang.org/x/net/html` (BSD-3, part of the Go project) join for HTML import? | **a.** Yes. **b.** No: `encoding/xml` in its lenient mode reads tidy HTML only, and real files often fail. | **a.** Parsing hostile HTML is exactly what must not be hand-written. |
| 3 | Who donates a Confluence export? | The importer can be built from public format descriptions and our own test files. Real exports have surprises that those do not show. It must be a third party's own data, and never your employer's. | Until a sample arrives, the importer is marked "tested on our own files only" in the docs and on its screen. |
| 4 | Which targets for public sites? | **a.** A folder and a zip, for now. **b.** Also S3-compatible storage, which needs storage work from phase 3 or a new module. | **a.** |

## Scope

**In:** EXC-004 to EXC-018, EXC-025 to EXC-046.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| EXC-019, print style | Built in wave B |
| Download all attachments of a page (FIL-035) | Wave E. This wave adds the same for a space (EXC-018) |
| PDF of a whole space without the helper process | Not possible, see D3 |
| Sites on S3 or over SFTP | Phase 3 at the earliest |
| Importers for closed formats without public documentation | Not planned |
| Moving a space with its users between linked instances | Phase 8. The bundle lays the ground |

## Feature plan

### G1. The export dialog

More, Export on a page, and Export in the space menu and Space settings. One dialog replaces the two Markdown entries.

- **Choices:** format, then what: this page, this page and below, or Choose pages. The list only offers formats that are switched on, and that the person may use.
- **Choose pages:** the tree as nested checkboxes. A parent selects its branch, and each page can be deselected. A filter narrows the list. Select all and Select none.
- **Options:** comments, old versions, attachments, restricted pages I can see. Only what the format can hold is offered: Plain text has no options, and Bundle has its own fixed content.
- **PDF extras:** paper size (A4, Letter), orientation, table of contents.
- **Keyboard:** native controls, in reading order.
- **Accessibility:** `fieldset` and `legend`, with the number chosen in a `role="status"` line.
- **Edge cases:** nothing chosen disables Export and says why. A person without *export* sees no entry. A draft-only page is left out and named.

### G2. Running and delivering an export

- **Run:** `POST /exports` starts a job. The dialog shows progress and can be closed. Done means a notification with a link (wave C).
- **File:** kept 24 hours in storage under its own key, then removed by a periodic task. An expired link says so, and offers to make it again.
- **Failures:** a page that fails is named in the result, the rest goes on, and the file carries a list of what is missing.
- **Limits:** one running export per person, and a maximum size set by the administrator.
- **Content:** published versions only (EXC-015). Sections are open (EXC-013). Dynamic blocks show what the exporting person sees (BLK-011), and pages they may not see stay out of them.
- **Audit:** `export.created` with format, root and page count, and `export.downloaded`.

### G3. Plain text and HTML

- **Plain text:** `document.Text`, one `.txt`, or a zip of them, with headings kept as lines.
- **HTML:** a zip that works offline: `index.html` with the tree, one file per page, `files/` for attachments, one style sheet from the design tokens with both themes. Links between chosen pages become relative links. A link to a page outside the set becomes plain text with a note in the report.
- **Safety:** no script in the export, and every value escaped.

### G4. Word and ODT

- **Mapping:** paragraphs, Heading 1 to 6 as real styles, bold, italic, underline, strike, code, sub and superscript, links, lists with nesting, task lists as text with a box character, quotes, code blocks in a mono style, tables with merged cells and a repeating header row, images with alt text, panels as a shaded one-cell table with their kind as a word, sections as a heading and text, columns as a table without borders, status marks and dates as text.
- **Colours:** names turn into the light value of the token.
- **A branch:** one file, with each page starting at a heading level below its parent, up to level 6.
- **Right-to-left:** a page in a right-to-left language sets `w:bidi` and its ODT equivalent.
- **Edge cases:** a file that is no longer readable by us is named. A very wide table keeps its width. An image of an unknown type becomes its file name.

### G5. PDF

- **Path:** `document.ToHTML` and a print style go to the helper process, with images as parts of the request. The helper has no network, and JavaScript is not needed.
- **Design:** a space administrator sets a cover (title, subtitle, logo), a header and a footer with `{page}`, `{pages}`, `{title}` and `{date}`, in Space settings. Size and orientation are chosen when exporting.
- **Contents:** a table of contents with page numbers for more than one page, and bookmarks from headings.
- **Language:** the document has a title and a language.
- **Without a helper:** the PDF choice is absent, and the administrator's export settings say what to install.
- **Edge cases:** a helper that fails or takes over five minutes fails the job with a clear sentence. A wide table scales down to the paper.

### G6. The bundle

- **Content:** `manifest.json`, `manifest.sig`, then the space, pages with all versions, comments, labels, templates, permissions, restrictions, statuses and files.
- **Who:** *administer* on the space. It always holds everything in the space, so it also needs *export* on it. A page the person cannot see stops the export with a sentence, because a bundle that silently misses pages would be worse.
- **People:** a list of names and e-mail addresses that the content refers to, and nothing else about them.
- **Size:** files are streamed, never held in memory.

### G7. Export rules for administrators

Administration, Export. A row per format: on or off, and optionally limited to groups. The server enforces it, and the dialog follows. Also the maximum size, and whether a helper is connected.

### G8. The import pipeline

Space settings, Import, and Create, Import. One screen for every kind.

- **Steps:** choose a file, read it, see the report, match people, run. Every import goes through the report, which is the dry run (EXC-033).
- **Report:** number of pages, files, comments and versions, the tree that will appear, warnings with the page they belong to, and what will not be carried over.
- **Match people:** a table of authors found, with e-mail where known. Matched ones are filled in. For the rest: pick a person, or keep the name as a former author. The default for the rest is the person importing.
- **Run:** a background job with progress, under a chosen parent. Pages keep their tree order.
- **Undo:** one button while nobody else edited or commented on the imported pages. If someone did, it says which pages, and offers to undo the rest.
- **Keyboard and accessibility:** a table with row headers and labelled selects. Progress in `role="status"`.
- **Limits:** the existing limits for Markdown. An administrator setting for larger imports, up to 10 GiB compressed.
- **Edge cases:** a file of the wrong kind says what it seems to be. An import stopped halfway keeps what it made, marks the run as failed, and Undo cleans it up. Two imports into one parent take turns.

### G9. Markdown, better

- **Links:** a second pass turns links between imported files into links to the new pages, also with a heading in them (EXC-025).
- **Front matter:** `title`, `labels`, `status`, `created`, `updated` (EXC-026). Unknown keys give a warning.
- **Wiki links:** `[[Page]]` and `[[Page|text]]` resolve by title within the import, for exports of other Markdown wikis (EXC-034).

### G10. Word

- **One file:** makes a page with headings, lists, tables, images and links. Create, Import offers it (PAG-005).
- **Several files:** become pages under a new holding page with a view restriction to the importer, so they can be reviewed first (EXC-028).
- **Into a draft:** Import Word in the editor's Insert list puts the content at the cursor (EXC-029).
- **Reading:** follow the relationships to the main part. Headings come from style names, in any language through `w:outlineLvl`. Embedded images become attachments.
- **Refused or dropped:** external relationships are never fetched. Macros, embedded objects, tracked changes (the result is taken) and comments are dropped, with a warning.
- **Edge cases:** a `.doc` file says it needs saving as `.docx` first. A password-protected file says so.

### G11. HTML

A zip of HTML pages and images makes a tree from its folders, or from an `index.html`. Every page goes through the same step as a paste: it becomes a document through the allow-list, and nothing else remains. Script, style and event handlers go. External images become links, and links between the files become links to the new pages.

### G12. Confluence

Import from a space export, from the format as publicly documented.

- **Reading:** stream `entities.xml` once to spool objects to disk with an index on id, then build relations from both sides. Bodies are parsed one at a time.
- **Which space:** `exportDescriptor.properties` and the objects decide. Several space objects are listed in the report, and the person picks one.
- **Carried over:** the page tree, order, titles, current body, attachments with their versions, comments with replies, labels, versions when chosen, and page restrictions and space permissions where the people or groups can be matched.
- **Versions:** on by default for up to 50 per page, because they make the import slow. Each keeps its author and date.
- **Permissions:** an unmatched restriction never disappears. The page is restricted to the person importing, and the report says so, so that nothing becomes more open than it was.
- **Body:** see the table below. Links to other pages in the export become links by id. Links to pages that are not there stay as text with a note.
- **Limits:** expanded size, depth and object count are checked while streaming.
- **On screen:** "Tested on our own files only" until a donated sample has passed.

| Storage format | Becomes |
|---|---|
| Paragraphs, headings, lists, tables, quotes, rules, bold, italic, underline, strike, sub and superscript, code | The same blocks and marks |
| `ac:link` with `ri:page` | A link to the new page, or text with a note |
| `ac:link` with `ri:attachment` | A file tile or a link to the file |
| `ac:image` with `ri:attachment` | An image, with alt text and size |
| `ac:image` with `ri:url` | A link, because we never show outside images |
| `ri:user` | A mention when the person is matched, otherwise the name as text |
| Task lists | Tasks, with people and dates where matched |
| Page layouts | Columns |
| Macros for info, note, tip and warning | Panels of the nearest kind |
| Macro for code | A code block with its language |
| Macro for expand | A collapsible section |
| Macro for table of contents | Our contents block |
| Macro for status | A status mark, with the nearest colour |
| Macro for children | Our child pages block |
| Anything else | A Note panel with the macro's name, its parameters and its text |

This table comes from public documentation. Every row gets checked against a donated sample before the docs may call it proven.

### G13. Other wikis

- **MediaWiki XML:** pages become a tree by `/` in their titles, or flat. Each revision can become a version with its author and comment. Wikitext is read for headings, lists, bold, italic, links, tables and preformatted text. Templates and anything else stay as visible text in a Note panel. Files are not in this format, and the report says so.
- **Markdown wikis:** G9 covers them.

### G14. Importing a bundle

Administration, Import a bundle. The signature is checked first, with the fingerprint shown. Then the same report and match of people. An id that already exists offers to import as a copy with new ids, keeping `origin_instance_id`. Files are checked against the manifest while reading. A key that exists gets a new one, chosen in the report. Audit: `bundle.imported` with signer, source instance and counts.

### G15. Public websites

Space settings, Public site.

- **Setup:** name, address the site will have, language, logo, one of the accent colours, and whether editors' names show (off by default).
- **Choosing pages:** a tree of checkboxes, or the whole space with a note that new pages are not added unless chosen. A page under a view restriction cannot be ticked, and says why. Drafts are not offered.
- **Preview:** before any build goes out, a list of what will be added, changed and removed, with the warnings: restricted pages left out, links that go nowhere, images without alt text, images that keep their metadata. The first publish needs the name of the site typed in.
- **Updates:** on publish of a chosen page, debounced by a minute; daily; or by hand. Each build is new, and goes live by one atomic rename. The last three stay, so a build can be rolled back.
- **The site:** own theme from the tokens, light and dark, tree navigation, contents list, a search box, and an index page that works without script. No comments, no author names unless chosen, no version history.
- **Links:** to chosen pages only. Others become plain text.
- **Search engines:** allowed gives a `sitemap.xml` and an open `robots.txt`. Blocked gives `Disallow: /`, `noindex` on every page, and no sitemap. The note says that blocking is a request, not protection.
- **Inside the wiki:** a chosen page shows a Public mark by its title, and the editor shows the same, so writers know (EXC-043).
- **Withdrawing:** Withdraw empties the target and leaves a page that says it was withdrawn. Removing a page from the list takes it out in the next build, which starts at once.
- **Accessibility:** skip link, landmarks, `lang`, focus styles, and the same contrast as the app.
- **Audit:** `site.created`, `site.changed`, `site.published` with counts, and `site.withdrawn`.
- **Edge cases:** a failed scrub stops the build, keeps the old site, and names the page. A target that cannot be written keeps the old site and tells the administrator. A page in the trash leaves the site.

### G16. A single public page

More, Public link, when the instance and the space allow it. It builds that one page as a static page, under an address with 128 random bits, on the same public host. The dialog shows the address, what is included, and Withdraw. A later publish does not update it until the person says so, because they approved one version. A restriction or a delete withdraws it at once. The admin console lists all of them, with who made each.

## API

| Route | Rule | What |
|---|---|---|
| `POST /exports` | `space.export` | Start an export |
| `GET /exports/{id}` | owner of the export | State and report |
| `GET /exports/{id}/file` | owner of the export | The file, checked again |
| `GET, PUT /spaces/{id}/pdf-design` | `space.administer` | Cover, header, footer |
| `GET, PUT /admin/export-policy` | permission `administrator` | Formats and limits |
| `POST /spaces/{id}/imports` | `page.create` | Upload, with `kind`. It now ends at the report |
| `GET /imports/{id}` | owner of the import | Report and state |
| `PUT /imports/{id}/mapping` | owner of the import | Match people, pick a space |
| `POST /imports/{id}/run` | owner of the import | Carry it out |
| `POST /imports/{id}/undo` | owner of the import | Undo |
| `POST /pages/{id}/import-word` | `page.edit` | Word into a draft |
| `POST /spaces/{id}/bundles` | `space.administer` | Start a bundle export |
| `POST /admin/bundles` | permission `administrator` | Import a bundle |
| `GET /admin/bundles/key` | permission `administrator` | Our public key and fingerprint |
| `GET, POST /spaces/{id}/sites` | `space.administer` | List and create sites |
| `GET, PATCH, DELETE /sites/{id}` | `space.administer` | One site |
| `PUT /sites/{id}/pages` | `space.administer` | Choose pages |
| `POST /sites/{id}/preview` | `space.administer` | What would change |
| `POST /sites/{id}/publish` | `space.administer` | Build now |
| `POST /sites/{id}/rollback` | `space.administer` | Back to an earlier build |
| `POST /sites/{id}/withdraw` | `space.administer` | Take it down |
| `GET /sites/{id}/download` | `space.administer` | The site as a zip |
| `GET, PUT, DELETE /pages/{id}/public-link` | `space.administer` | A single public page |
| `GET /admin/public-links` | permission `administrator` | All of them |

`TestEveryObjectRouteIsProtected` needs ids for `/exports/`, `/imports/` and `/sites/`. No route is public: the instance serves nothing to the world.

## Data

Migration 0017, expand only (`page_versions` also gains `imported`, and `export_run_pages` holds what a file holds):

- `export_runs`: job, space, format, options, object key, file name, size, created by, expiry.
- `export_policy`: format, enabled, group ids.
- `space_pdf_design`: space, cover, header, footer, logo attachment.
- `import_runs`: space, parent, kind, state, stored plan, report, people map, created by, times.
- `import_items`: run, source reference, created page, version made. Undo and link rewriting read this.
- `public_sites`: space, name, address, target, theme, names shown, search engines, update mode, last build.
- `public_site_pages`: site, page.
- `public_site_builds`: site, number, state, counts, report, folder, times.
- `public_links`: page, version, token, created by, created at, withdrawn at.

The settings gain `public_sites_enabled`, `public_links_enabled` and `max_import_mib`. The target folder is configuration, not a setting, because a person must not be able to write anywhere on the server. `DIFFLUENCE_PUBLISH_DIR` holds it, and a site's target is a name below it.

## Build order

1. `document.ToHTML`, with tests against every block.
2. Export runs, the dialog, plain text and HTML.
3. Word and ODT writers.
4. PDF, if the product owner chooses a helper.
5. The `Plan`, the report, match people, Undo, moving Markdown onto it.
6. Word, HTML, wiki links, front matter.
7. Confluence reader and body conversion, on our own files.
8. MediaWiki reader.
9. Bundle out and in.
10. Sites: builder, scrub, preview, updates, marks.
11. Public links.
12. Docs, status lines, handoff.

## Test plan

| Level | What |
|---|---|
| Go unit | `ToHTML` escapes everything and knows every block. Body conversion table, row by row. Front matter. Wikitext. |
| Fuzz, nightly | Storage format, DOCX reader, ODT reader, front matter, wikitext, bundle manifest |
| Hostile files | Zip bomb, path out of the archive, nested zip, `DOCTYPE` with entities, deep XML, external relationship in DOCX, HTML with script and event handlers, wrong file under the right name |
| Round trips | Each block through DOCX and ODT and back. Bundle out and in, with versions, comments and labels the same |
| Go integration | Exports leave out a hidden page. Only the owner downloads. An expired file is gone. A restriction after the start of a job. Formats limited to a group. Undo refused after an edit. An unmatched restriction stays closed. A changed byte in a bundle. An unknown signer. |
| Public site | The scrub finds a planted address. A restricted page never appears. A page deleted leaves at once. A file not shown stays out. JPEG metadata is gone. Sitemap and `robots.txt` in both modes. An atomic switch under a failing build |
| Route walk | New object routes join by themselves |
| Mutation | A hidden page in an export. Download by another person. A bundle with a wrong signature accepted. A restricted page on a site. Metadata kept. The scrub off. Undo over an edited page. An external relationship followed |
| Browser | The dialog by keyboard. Choose pages. Import with a report and match of people. Undo. Publish preview. The built site with axe, and search by keyboard |
| Pictures | ExportDialog in its screens, ImportView, SitesView, Public mark, the site itself in light and dark |
| PDF | A fake helper in Go tests. The real one nightly, checked for tags and title |
| Word | Our reader in CI. LibreOffice opens our files in the nightly run, when the helper is there |

## Acceptance criteria

- [ ] AC-1 One page, a branch and a chosen set export as plain text, HTML, Word and ODT, and PDF when a helper is there
- [ ] AC-2 An export never holds a draft, or a page the person may not see at that moment
- [ ] AC-3 A long export runs in the background, ends with a notification, and its link works once for that person for 24 hours
- [ ] AC-4 A failing page is named, and the rest arrives
- [ ] AC-5 An administrator can turn a format off, or limit it to a group, and the server refuses it
- [ ] AC-6 Word and ODT files have real headings, table headers and alt text
- [ ] AC-7 The HTML zip works offline, with no script
- [ ] AC-8 A bundle moves a space to a second instance with versions, comments, labels and permissions
- [ ] AC-9 One changed byte in a bundle stops the import, and an unknown signer needs a fingerprint
- [ ] AC-10 Every import shows a report first and changes nothing until confirmed
- [ ] AC-11 People are matched on e-mail, and the rest can be chosen
- [ ] AC-12 An import can be undone in one action, and never removes someone's edit
- [ ] AC-13 Links between imported files become links to the new pages
- [ ] AC-14 Word files become pages, several go under a restricted holding page, and one can go into a draft
- [ ] AC-15 HTML goes through the allow-list, and no script survives
- [ ] AC-16 Our Confluence test files arrive with tree, files, comments, labels and versions, and unknown macros stay readable
- [ ] AC-17 A restriction that cannot be matched leaves the page closed
- [ ] AC-18 MediaWiki XML becomes pages
- [ ] AC-19 A public site holds exactly the chosen pages, and the preview said so first
- [ ] AC-20 No instance address, user e-mail or hidden page is in the built site
- [ ] AC-21 A page that gets a view restriction leaves the site at once
- [ ] AC-22 Search on the site works, the index works without script, and both search engine modes are right
- [ ] AC-23 Chosen pages show a Public mark in the wiki
- [ ] AC-24 A single public page can be made and withdrawn, when both switches allow it
- [ ] AC-25 Every step is audited, and axe finds nothing on the screens or on the site

## Risks

| Risk | Answer |
|---|---|
| Real Confluence exports differ from what is public | The screen says what it was tested on. Unknown content stays visible. A donated sample turns into test rows, kept out of the repo |
| Our own DOCX trips up Word | Keep to the plainest parts, test with an independent reader, and the product owner opens one in Word |
| A secret reaches the public site | Pages by hand only, no restrictions, the scrub, and a preview. Withdraw is one action |
| Metadata stays in GIF and WebP | The preview warns, and the person decides |
| A huge import fills the disk | Limits while streaming, and a check of free space first |
| A bundle carries personal data around | It is an export with a reason: it needs *administer*, is audited, and lives 24 hours |
| The helper for PDF is a large image | It is optional, and the product owner decides |
| A rollback brings back a page that was restricted later | A rollback runs the current rules over that build, and removes what no longer may be public |

## How the product owner tests it

1. Open a page with sub-pages, choose More, Export, and pick Word. Open the file in Word and in LibreOffice.
2. Choose Export again, pick HTML and three pages by hand. Unpack the zip and open `index.html` with no network.
3. Export a Markdown zip, and import it into another space. Read the report, match a person, run it, and press Undo.
4. Import a Word file through Create. Then import three at once, and see that they wait under a restricted page.
5. Export a space as a bundle, start a second instance, and import it. Change one byte in the file, and try again.
6. Make a public site with three pages. Read the preview. Publish, and open the folder with a browser. Try search with the keyboard.
7. Put a restriction on one of those pages, and watch it disappear from the site.
8. Make a public link for one page, open it in a private window, and withdraw it.

## Provenance

`docs/blueprint/03-functional/11-import-export-publishing.md` and its sources, ADR-010, and `docs/blueprint/02-clean-room-and-licensing.md`. Read on 2026-11-05, for behaviour and format only, in own words:

- Atlassian's public page on the Confluence storage format (`confluence.atlassian.com/doc/confluence-storage-format-790796544.html`), and public issue CONFCLOUD-71403 on `jira.atlassian.com`.
- XWiki's public documentation pages on the Confluence XML format (`xwiki.org`). Documentation only, no source code.
- Microsoft Learn on the structure of a WordprocessingML document, and the Library of Congress format descriptions for DOCX and ODT. ISO/IEC 29500 and ISO/IEC 26300 as referenced there.
- OASIS and `opendocument.xml.org` on OpenDocument.
- MediaWiki's Help:Export.
- The Sitemaps protocol at `sitemaps.org`, and MDN on the robots meta tag.
- PDF Association and iText pages on tagged PDF from Chrome, and on PDF/UA.
- W3C WCAG 2.2 Understanding 1.1.1 and 2.2.1, and the WAI-ARIA tree view pattern.
- Gotenberg's public repository page, for what it is and its licence.

No source code of any other wiki was read, and no Atlassian product or account was used. Own design.
