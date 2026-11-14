# 11.10: Screens

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

The screens follow the design standard (`docs/design/README.md`): the Stil look of ADR-034, the **Settings** frame for the administration, the listed components, every word through `en.json`, every new view with an approved picture in `web/e2e/0-design.spec.ts` in light, dark, narrow and right-to-left, and WCAG 2.2 at level AA. This study adds **no new frame and no new component**: new sections in the group System, and lines in sections that exist. No layout, icon or wording of another product is used.

## 10.1 Where things live

The administration's group **System** (ADR-034) holds today: System health, Backups, Configuration, Care, Settings, Mail, Audit, Insights, Analytics. Sessions sits under People and access.

- **CSL-800** · new · Three new sections in System, each at its own address (`?section=`): **Upgrades**, **Worker** and **Events**; and one for the key, **Encryption key**. Order in the group: System health, Backups, Worker, Upgrades, Configuration, Encryption key, Audit, Events, Care, Settings, Mail, Insights, Analytics. *Why:* the running of the installation together, then the rest as it is. *Proof:* the design test of the sidebar.
- **CSL-801** · new · An experimental section that is off is **absent**, not disabled ("No permission: the control is absent"); its place in Administration, Organising, Experimental parts says what turning it on adds. *Proof:* an e2e test with each part off.
- **CSL-802** · new · **Jump to a setting** finds the new sections and the settings inside them (log level, disk thresholds, restore test time), as it finds the others.

## 10.2 The patterns every section uses

- **CSL-805** · new · **One main action per view** as `<button className="primary">`; the rest `<button>` or `ghost` in tables. A tier-2 or tier-3 action that removes or rewinds is `className="danger"` and asks first with `ConfirmDialog`, whose button repeats the action ("Retire the old key"). *Why:* design standard, Components and Words.
- **CSL-806** · new · **The code from the worker's log** uses the restore's existing form (`RestoreCode` in `admin-running.tsx`), moved into `components/` as `WorkerCode` and used by every tier-3 task: the sentence "Enter the confirmation code from the worker's log", the platform's command to read it (CSL-320) in a code line, a `Field`, tries left, and Cancel. *Why:* "Need something new? Add it once in `components/`". *Proof:* the existing restore pictures stay equal; a picture of the component on another task.
- **CSL-807** · new · **A fresh login and a second factor** use the existing re-authentication dialog; nothing new.
- **CSL-808** · new · **Progress** of a task is a `role="status"` line in words ("Resealed 1,200 of 5,000 values"), never only a bar. *Why:* WCAG 4.1.3.
- **CSL-809** · new · **Every table** (backups, tasks, nodes, keys, migrations) is a `TableBox` around a plain table, or `SortTable` where people sort; rows, not boxes; states as `.lozenge` with words; times as `RelativeTime`.
- **CSL-810** · new · **A waiting change** (lowered retention, a tier-3 task awaiting its code) is a `.note` at the top of its section with the date it takes effect and Cancel. *Why:* "A message in the flow".
- **CSL-811** · new · **The states of the standard** in every section: Loading; Failed with `ErrorNote`; Empty as a muted sentence that says what will appear ("No task has run yet. Work you ask the worker for shows here."); No permission as absence; Busy with the button disabled. **No worker** is its own muted sentence with the platform's way to add one and the command for the action (CSL-321).

## 10.3 Each section

- **CSL-815** · new · **Backups** keeps its order (worker state, Back up now, schedule, list, test restores, restore) and gains: the target as a native `select` of the reported targets with each one's free space (CSL-503); the restore test's time and **Test now** under the test restores (CSL-512); in the list, the target and "needs the previous backup key" as a `.lozenge` (CSL-506). Lowering the number kept opens the tier-3 flow in place. *Proof:* pictures with one target and two, and with a waiting tier-3 change.
- **CSL-816** · new · **Worker**: a `TableBox` of workers (platform, version, last seen, capabilities in words, targets), then the tasks as a `SortTable` newest first with a `ghost` Cancel per open row. *Proof:* a picture with a running task and a failed one.
- **CSL-817** · new · **Upgrades**: `.tiles` with this release, the newest node's release, expand migrations waiting, contract migrations waiting; then the nodes and workers table; then the contract migrations with the grace's end and, when allowed, **Run them now** (danger, tier 3); then "How to upgrade" with the platform's commands in a code block. *Proof:* pictures with nothing waiting and with a contract migration waiting.
- **CSL-818** · new · **Configuration** gains `Tabs` with one tab per process (CSL-602), the mark "changes at the next restart" as a `.lozenge` with words, and the mark "differs between nodes". *Proof:* a picture with two processes.
- **CSL-819** · new · **Encryption key**: the five steps as an ordered list, each with its state in words and time; only the next step has a button. The nodes and the key ids each holds below, in a table. On a read-only mount each step shows its command instead of a button. *Proof:* pictures at each step and on a read-only mount.
- **CSL-820** · new · **Audit** gains, under Check: the newest checkpoint, the signing keys in a table, the witness's last result and age; and the retention with its preview and the waiting change. *Proof:* a picture after a change point with a witness reported.
- **CSL-821** · new · **Events**: the state as sentences (transport, drain, older nodes, the release without NATS), then **Switch to PostgreSQL** or **Switch back to NATS** as the one main action. *Proof:* pictures under each transport.
- **CSL-822** · new · **System health** gains two downloads, "Monitoring rules" and "Dashboard", as `MenuDownload` items in a `Menu` beside the checks (CSL-585, CSL-586), and the log level as a line at its foot: "Every node logs at info", or "Debug until 15:00 on all nodes" with **Change** opening a `Dialog` with the level, the time and the nodes. *Why:* the log level and the files serve what health shows. *Proof:* pictures with and without an override.
- **CSL-823** · new · **Settings** gains a section **Health** with the two disk thresholds as number `Field`s with their bounds in the help (CSL-581).
- **CSL-824** · new · **Sessions** gains the count in the dialog that logs everyone out (CSL-541), and the link to the incident steps (CSL-542).

## 10.4 Words

- **CSL-830** · new · **One word per concept**: "worker" for the process that does the work on every platform (the backup service in Compose is "the worker" in the console, and its section says "In Docker Compose this is the backup service"); "task" for a request to it; "code from the worker's log" for the confirmation; "encryption key" for `master.key` in headings, with the file name in the text; "backup key" stays as today.
- **CSL-831** · new · **Plain sentences for each tier-3 request** that say what is lost: "This removes 12 backups, the oldest from 3 November. You cannot get them back." *Why:* "Errors say what happened and what to do", and the person must know what they confirm.

## 10.5 Accessibility

- **CSL-835** · new · Every flow works by keyboard, focus returns to the button that opened a dialog, codes can be pasted with spaces or dashes, and no meaning rests on colour alone. *Proof:* axe in every new picture test; a keyboard test of the tier-3 flow.
- **CSL-836** · new · Narrow screens: the tables scroll in their `TableBox`, the tiles wrap, and the code field keeps its full width. *Proof:* the narrow pictures.
