# 03.8: Finding and organising

## 8.1 Quick search

- **ORG-001** · built · Selecting the search box, or pressing `/`, opens a panel under it. Before typing, it shows recently visited pages and recent searches.
- **ORG-002** · built · While typing, it shows the best matches in groups: Pages, Blog posts, Files, Spaces, People. Each row has a title, the space and the date of the last change.
- **ORG-003** · built · Results appear within 300 ms of a pause in typing, and earlier answers never overwrite later ones.
- **ORG-004** · built · Arrow keys move through the rows. Enter opens the selected row, or goes to the full results when none is selected.
- **ORG-005** · built · The last row is always **All results for "…"**.
- **ORG-006** · built · Inside a space, a chip limits the search to that space, and can be removed with Backspace.
- **ORG-007** · built · A person can clear recent searches.

## 8.2 The results screen

- **ORG-008** · built · Search covers titles, published content and attachment names.
- **ORG-009** · built · Results show the title, a snippet with the matching words marked, the space and the date.
- **ORG-010** · built · Filters: space, author, date.
- **ORG-011** · built · Results never include anything the person may not see. Search is part of the permission tests.
- **ORG-012** · built · The last word also matches as a prefix.
- **ORG-013** · built · Publishing updates the search index in the same transaction, so a page is never found with stale content or permissions.
- **ORG-014** · built · Dutch and English behave alike, because no stemming is applied.
- **ORG-015** · partly · Filters added: content type, label, contributor (wrote, edited or commented), status, below a page, titles only. Still open: a control for "below a page", which works through the address.
- **ORG-016** · built · A filter on date offers last day, week, month, year, or a range.
- **ORG-017** · partly · Several people or labels in a filter mean **or**. Still open: several people, which work through the address.
- **ORG-018** · built · Results can be sorted by relevance or by date.
- **ORG-019** · built · Breadcrumbs under each result show where it lives, and are links.
- **ORG-020** · built · Archived spaces and pages are left out unless a filter includes them.
- **ORG-021** · built · Filters are part of the address, so a search can be shared or bookmarked.
- **ORG-022** · built · Results load in pages, with a total count.
- **ORG-023** · built · A search with no results suggests removing filters, and shows a similar spelling when there is one.
- **ORG-024** · built · Comments are searchable, and a result links to the comment.
- **ORG-025** · later · The text inside attachments (PDF, Word, text) is searchable when extraction is turned on. It runs in the background, in a separate process, with limits.
- **ORG-026** · built · **Search people and spaces:** results also list people and spaces, in their own tabs.
- **ORG-027** · built · An administrator sees which searches found nothing (PLT-014), with no names attached.

## 8.3 Search language

- **ORG-028** · built · Words in double quotes match that exact phrase.
- **ORG-029** · built · `AND`, `OR` and `NOT`, in capitals, and brackets for grouping.
- **ORG-030** · built · `?` stands for one character and `*` for any number, not at the start of a word.
- **ORG-031** · built · Field filters such as `title:plan`, `label:hr`, `space:ENG`, `author:anna` also work in the box.
- **ORG-032** · built · A search that cannot be understood falls back to plain words. It never gives an error.
- **ORG-033** · later · Search in other languages improves with stemming per space language, as a setting.

## 8.4 Labels

- **ORG-034** · built · A person with *edit* adds labels to a page, while reading or editing. Labels are words that group pages across the tree.
- **ORG-035** · built · Labels are lower case with no spaces. Capitals become small letters and spaces become dashes while typing.
- **ORG-036** · built · Typing suggests existing labels. Enter adds one, and several can be added in a row.
- **ORG-037** · built · Labels show at the bottom of the page, and each is a link to a screen with all pages that have it.
- **ORG-038** · built · A label screen can be limited to a space, and can combine labels: pages that have both.
- **ORG-039** · built · A list shows all labels of a space or the instance, alphabetically or by popularity, with the number of pages.
- **ORG-040** · built · A space administrator can rename or merge labels in a space, in one action. It is audited.
- **ORG-041** · built · A person with *edit* removes a label from a page with one click. A label that no page uses any more disappears.
- **ORG-042** · built · Labels also apply to blog posts and attachments.
- **ORG-043** · built · An administrator can define an optional fixed list per space. Then only those labels can be used.
- **ORG-044** · built · Removing or adding a label does not create a page version, but it shows in the activity of the page and in the audit log.

## 8.5 Templates

- **ORG-045** · partly · An empty page offers templates beside the editor: a gallery with a search box, categories, a preview and a description. Choosing one fills the page. Still open: the gallery opens as a dialog from a note above the editor, not as a panel beside it.
- **ORG-046** · built · A space administrator makes templates for a space. An administrator makes them for all spaces.
- **ORG-047** · built · A template is written in the same editor as a page.
- **ORG-048** · built · A template can hold **labels**, which new pages receive, and **page facts** (BLK-039).
- **ORG-049** · built · A template can hold **instruction text**, which shows only in the editor and disappears when typing starts. Readers never see it.
- **ORG-050** · built · A template can hold **fields**: single line, multiple lines or a list of choices. Creating a page first shows a short form, and the answers fill in the page. Fields with the same name share a value.
- **ORG-051** · partly · A template can set the title pattern, such as `Meeting {date}`, and a default parent page. Still open: a field for the default parent. The server keeps it.
- **ORG-052** · built · Templates can hold images and attachments, which are copied to each new page.
- **ORG-053** · built · Diffluence ships a starter set in our own words: meeting notes, decision record, project plan, how-to guide, incident review, retrospective, requirements, onboarding plan, team home, weekly update.
- **ORG-054** · built · Administrators can hide starter templates, and promote templates so they come first.
- **ORG-055** · built · A **space template** bundles a home page, pages, labels and templates for a new space.
- **ORG-056** · built · A person can favourite templates, and sees recently used ones first.
- **ORG-057** · built · A template can be saved from an existing page.
- **ORG-058** · built · Changing a template does not change existing pages. Deleting one does not either.
- **ORG-059** · partly · Templates can be exported and imported between instances. Still open: files in the exported file.
- **ORG-060** · partly · Templates are versioned like pages, with the same comparison. Still open: the screens for the versions of a template. Each save is a version.
- **ORG-061** · partly · **Guided page:** a template can start with a short wizard of several steps, which fills in the page and sets the labels and parent. Still open: writing steps on the template screen, and a last step for labels and parent.

## 8.6 Blog posts

- **ORG-062** · built · A space can have a blog: dated posts outside the page tree, for news and updates.
- **ORG-063** · built · A post uses the same editor as a page, including drafts, versions, comments, reactions, labels and restrictions.
- **ORG-064** · built · A post has a title, an author, a publication date and an optional header image.
- **ORG-065** · built · The Blog entry of the sidebar lists posts newest first, with title, author, date and excerpt, grouped by year and month.
- **ORG-066** · built · A post's address contains its id, and stays stable.
- **ORG-067** · built · A page can be converted to a post before it is first published, and back.
- **ORG-068** · new · A person can follow all posts in a space, or all posts everywhere.
- **ORG-069** · built · A post can be scheduled, like a page.
- **ORG-070** · built · A site-wide blog screen lists posts from all spaces a person may see.

## 8.7 Hub pages and structure

- **ORG-071** · built · **Folders:** a tree entry with no content of its own, to group pages, and which has its own restrictions. A page can be turned into a folder and back.
- **ORG-072** · built · **Link entries:** a tree entry that points to an address, shown with an icon. Useful for bringing outside tools into the tree.
- **ORG-073** · built · **Company home:** an administrator can publish a page as the home of the instance, with cards, links and news. It shows for everyone at Home, with an option to go to the personal start screen.
- **ORG-074** · built · **Reorder a whole level:** the Content screen can sort all children of a page by title or date in one action.
