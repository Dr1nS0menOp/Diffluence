# 09.T14: What this feature will not do

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

Each line is something a reader might expect from the name "guided templates", left out on purpose, with the reason. **Later** means it may come in its own study; **no** means it is not planned.

- **TPL-1200** · no · **Scripting** of any kind in templates: no code, no expression language, no template language such as Jinja in the text. *Why:*
  - the roadmap asks for "without scripting";
  - a script is hard to review at approval;
  - document assembly that uses code makes authors reason about re-running and side effects (docassemble).

  Structured conditions (chapter 06) cover the situations of chapter 02.
- **TPL-1201** · later · **Formulas and nested logic:**
  - computed values ("start date plus two weeks" as a formula, sums, joined text);
  - conditions nested inside each other;
  - comparisons between two fields (TPL-434).

  *Why:* each step toward these is a step toward a language. They come back only with a need that fixed choices cannot meet. An embeddable, non-Turing-complete language such as CEL exists for that day, and its licence fits (Apache 2.0).
- **TPL-1202** · later · **A page per item of a list**: one page per team chosen, one per person in a People field (decided, TPL-1418). *Why:*
  - it turns a set of known size into one of unknown size;
  - the check step would have to show it all, and the limits of chapter 10 would need a second count.

  The planning-round situation of chapter 02 waits for it.
- **TPL-1203** · no, for now · **Merging space templates and sets** (decided, TPL-1420). A space template makes a new space; a set adds pages to a space that exists. A later study may let a space template hold guided templates, which ORG-055 already promises for templates in general. *Finding:* `StartSpace` is not all or nothing (TPL-008), and should be fixed on its own.
- **TPL-1204** · no · **Pages that stay linked** to their template and change when it changes, or an action that "updates this page to the newest version" (decided, TPL-1416). *Why:* a page is people's work from the moment it exists (ORG-058). Live content is what dynamic blocks and excerpts are for.
- **TPL-1205** · later · **Rights from answers**: a field that decides who may see or edit a page, such as "only the manager chosen". *Why:* it would make a form answer a way to grant access. If it comes, it needs `restriction.manage` and its own tests in the permission matrix.
- **TPL-1206** · later · **A library of variables** shared by many templates, or for the instance (decided, TPL-1419). *Why:* it needs its own rights, versions and approval, and no situation needs it yet.
- **TPL-1207** · no · **Fields that hold files, rich text, signatures or secrets.** *Why:*
  - files belong in the page once it exists;
  - rich text would let an answer carry markup (spec 011);
  - signatures belong to document control (09.8);
  - secrets never belong in a wiki.
- **TPL-1208** · no · **Forms that send data elsewhere**: a template is not a survey tool, and its answers go into pages and nowhere else. No webhook carries answers (TPL-723).
- **TPL-1209** · no · **Sets across spaces** (decided, TPL-1402). *Why:* "all or nothing" across several spaces' rules and locks is much harder to keep true, for a need nobody named.
- **TPL-1210** · later · **Blog posts and databases as entries.** *Why:* posts sit outside the tree, and databases are an experimental part of their own (ADR-043). They may join when databases leave the experimental list.
- **TPL-1211** · no · **Approval through a document-control workflow** (decided, TPL-1413). *Why:* one decision needs no workflow; a team that wants more can keep templates as controlled pages first (TPL-522).
- **TPL-1212** · later · **Regular expressions** in validation (decided, TPL-1408), if a need appears that the named formats cannot meet. It would be one syntax (RE2, which Go uses and which runs in linear time), checked on the server only.
- **TPL-1213** · no · **Draft and published spaces**: their own study, in parallel. This study only notes that a set made in a draft space is an ordinary set there.
- **TPL-1214** · later · **Reports on template use** beyond the counts of TPL-613, such as which fields people leave empty. *Why:* useful to template authors, but it would mean keeping answers, which this feature does not (TPL-1409).
