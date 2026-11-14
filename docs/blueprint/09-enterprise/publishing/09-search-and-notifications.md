# 09.P9: Search, the feed and notifications

How to read this chapter: see [01-draft-and-published-spaces.md](01-draft-and-published-spaces.md).

Search is where people spend much of their time (ADR-035), and notifications are a distribution channel. Both already follow the rule "a person sees what they may see", per space. The pair adds two questions: how a person who sees both spaces tells the copies apart, and how a release tells readers without a flood.

## 9.1 Search

- **PUB-800** · new · Draft pages and twins are indexed as the pages of their own spaces, with no new index (decided, PUB-1424). A reader finds twins; a writer who may view both finds both. *Why:* the existing rule already keeps draft words from readers (PUB-622). *Proof:* the test of PUB-622; a test for a person in both spaces.
- **PUB-801** · new · A result from a **draft space** carries the word **Draft** beside the space's name, as text, for people who may view that space. *Why:* someone who sees two results with one title must know which one readers have. *Proof:* an e2e picture of results with both.
- **PUB-802** · new · The search panel's filters gain nothing new: `space:` already chooses one space. *Why:* the search syntax stays one language (spec 033). *Proof:* none needed beyond existing tests.
- **PUB-803** · new · A twin is indexed in the **same transaction** as its version, as every version is (spec 003), so in a one-transaction run (PUB-450) search changes together with the pages. In a batched run each batch's twins become findable with that batch. *Proof:* a test that searches during a run.
- **PUB-804** · new · Trigram search on titles and file names (ADR-038) follows the same per-space rule; a title changed only in the draft is not found by readers. *Proof:* a test.
- **PUB-805** · later · Folding a draft result under its twin for people who see both, if use shows people are confused by two results.

## 9.2 The feed

- **PUB-810** · new · The published space's feed shows **one item per run**, "N pages released", with the release note and a list of the pages, not one item per page. A single-page run shows as that page's change. *Why:* a whole-space release of hundreds of pages must not bury the feed. *Proof:* a feed test with a run of 500 pages.
- **PUB-811** · new · The draft space's feed shows runs to those who may see the release screen, and is otherwise unchanged. *Proof:* a test.

## 9.3 Notifications

- **PUB-812** · new · **Mentions** in a released version notify the mentioned person, as a publish would, when they may view the twin and were not mentioned in the twin's previous version. The notification names the releaser as the actor and links to the twin. *Why:* a mention in published text is meant for its reader; one already mentioned need not be told again for each release. *Edge cases:* the same person was usually already told in the draft space if they may view it; that is two notifications for two spaces, which is honest. *Proof:* tests.
- **PUB-813** · new · **Watchers** of a twin, and watchers of the published space, are told of a run **once per person per run**, in one message listing the pages they watch that changed (decided, PUB-1428). *Why:* a mail per page for a whole-space release would be the flood that makes people stop watching. *Proof:* a test that a run of 300 pages sends one notification to a person watching the space.
- **PUB-814** · new · Notifications about a twin hold only twin content: its title, its excerpt, the release note. They are built through the published space's visibility, never from the draft page. *Proof:* a test that renders every notification kind about a twin for a reader.
- **PUB-815** · new · **Digests** treat a run as one item. *Proof:* a digest test.
- **PUB-816** · new · Watchers of a **draft** page keep receiving its changes as today; a release adds nothing for them except to the releaser (PUB-822). *Proof:* a test.
- **PUB-817** · new · **Tasks** in a twin are read-only and are not in anyone's Tasks screen; assigning a task in a draft page notifies only through the draft page (decided, PUB-1423). *Why:* a task belongs where it can be ticked. *Proof:* a test that a released task list creates no task rows for the twin.
- **PUB-818** · new · Taking a page off tells the twin's watchers once per run that it was taken off, without the reason unless it is in the release note. *Proof:* a test.

## 9.4 Who is told about runs

- **PUB-820** · new · An approval release (08) tells the approver and the page's authors in the draft space that the page was released, or why it was not. *Proof:* a test.
- **PUB-821** · new · A run can be **silent**: no notifications to watchers or mentioned people, as a silent publish today; the feed still shows it. *Why:* a typo fix should not call everyone; silent only affects notifications, as DOC-311 says for control. *Proof:* a test.
- **PUB-822** · new · The person who started a background run is told when it ends, in the app and by mail when they chose mail, with a link to the report (PUB-453). A run that stopped (PUB-454, PUB-1061) is told as well, to the releaser and to those with `space.release` who watch the draft space. *Proof:* tests.
- **PUB-823** · new · Notifications of this feature use the existing channels, outbox and rate limits; they add no new channel (as DOC-609). *Proof:* no new sender (review).

## 9.5 Webhooks and the API

- **PUB-830** · new · Webhook subscriptions of the published space receive `page.released` (per page, with the twin's id and version) and `release.finished` (per run, with counts); never a draft id. Subscriptions of the draft space with the right to see releases also receive the run with both sides' ids. *Why:* webhooks are a distribution channel. *Proof:* tests per subscription.
- **PUB-831** · new · The API offers every action of this feature with the same checks as the screens: link, end, settings, statuses, preview, run, cancel, leave out, include, take off, report, verify. *Proof:* API tests per action.
- **PUB-832** · new · Tokens can release, as people can, with the token owner's rights at the moment of the run; automation rules can release too (once automation is on, ADR-043), with the rule owner's rights. *Why:* a release at the end of a build pipeline is common in docs-as-code (02). *Edge cases:* a token that may release cannot bypass the preview: the API takes the preview's id, so a run is always of a preview someone asked for. *Proof:* tests.
- **PUB-833** · new · `GET` of a twin through the API returns, to callers who may view the draft page, the draft page's id and the draft version it came from; to others, nothing about the draft. *Proof:* API tests per kind of caller.
