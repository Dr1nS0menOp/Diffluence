# 09.T13: Migration from today's templates

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

Teams already use templates: their own, the ten starters, space templates, and the automation action that makes a page from a template. This feature must not break any of them, must not change what a person sees in the gallery without an administrator's choice, and must run beside the previous release on the same database during an upgrade. The upgrade machinery applies as always: expand and contract migrations, the one-day grace of wave O2, and the backup rule of ADR-040 and ADR-042.

- **TPL-1100** · new · The migrations are **expand only**. They add:
  - `template_versions`, `template_rights`, `template_spaces`, `template_reviews`, `template_sets`, `page_template_origin` and the pending copies of TPL-821;
  - nullable or defaulted columns on `templates` (`owner_id`, `state` defaulting to approved, `approved_version`);
  - the permission `template.propose` (TPL-061);
  - the space and instance setting of TPL-511.

  Nothing is renamed or dropped. *Why:* the upgrade promise of blueprint 07: the previous release keeps working on the new schema. *Proof:* the upgrade test across the release; today's template tests run unchanged after the migration.
- **TPL-1101** · new · Every template that exists is **Approved**, owned by its creator (`created_by`), with **version 1** made from its current row and the current version of its hidden page (TPL-620). The starters are approved and keep their keys. *Why:* what people can use today, they can use tomorrow. *Proof:* a test that the gallery of every space lists the same templates, in the same order, before and after.
- **TPL-1102** · new · `template.propose` goes to every role that holds `page.create`, so no one loses anything (decided, TPL-1410), with the migration proof of wave PM. *Proof:* the adoption test, which shows that no decision changes except the new permission.
- **TPL-1103** · new · The **previous release**, running beside the new one during the grace period, knows nothing of manifests, sets, new field kinds or conditions. If it met a guided template, it would drop what it does not know: unknown field kinds fall back to one-line fields (`fieldKinds`), and unknown blocks are lost on save. Two rules keep it from meeting one:
  - the experimental part starts **off** after the upgrade (TPL-050);
  - turning it on is **refused while the grace period** of the upgrade that brought this feature is running, with a message that says when it can be turned on.

  The previous release can still save a **plain** template the old way: it updates the row and the hidden page, and writes no manifest. The new release treats a plain template whose newest page version is newer than its newest manifest as changed by the older release. It makes the next version from the row and that text on first read. *Why:* an old release must never save or use a guided template with its parts dropped. Later upgrades meet two releases that both know guided templates. *Proof:*
  - the N/N+1 upgrade test with a plain template saved through the old release;
  - a test that turning the part on during the grace period is refused.
- **TPL-1104** · new · The lines of today's templates that are still **partly** built are finished in GT2, whether the experimental part is on or off:
  - ORG-045, the gallery as a panel beside the editor;
  - ORG-051, a field for the default parent;
  - ORG-060, the screens for versions;
  - ORG-061, writing steps on the template screen, and the last step for labels and parent.

  *Why:* they complete a part that exists (ADR-043 freezes new parts, not unfinished ones). *Proof:* the acceptance tests of those lines.
- **TPL-1105** · new · **Space templates** (ORG-055) are not changed (TPL-1203). **Automation's** `new_from_template` keeps working. A rule that uses a template which later gains a required field with no default fails its run with "This template asks for *field*, which a rule cannot answer", and the rule editor warns about such a template when it is chosen. *Proof:* a test of the failing run and of the warning.
- **TPL-1106** · new · **Exported** template files from before this feature import as one-entry templates of version 1. Files exported by this feature carry a format number, and an older release refuses them as a whole instead of importing part of one. *Proof:* a round-trip test across both formats.
- **TPL-1107** · new · Turning the experimental part **off** after use keeps every template, version, approval and origin record. Guided templates leave the gallery (TPL-050), and plain ones stay. Turning it on again brings them back as they were. *Proof:* a test of off and on again.
- **TPL-1108** · new · A later release may **contract** by dropping the fallback of TPL-1103 and the refusal during the grace period, once no supported release writes templates without a manifest. *Proof:* covered by the contract migration test when it is written.
