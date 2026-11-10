# 08.1: The road to 1.0 and beyond

Blueprint 08 sets the phases, ADR-024 put all of blueprint 03 first, ADR-028 placed migration last, and ADR-038 put the enterprise document features before 1.0. This chapter asks what research and comparable projects say about the order of such work, and what "1.0" should mean for Diffluence.

## 1.1 What the research and practice say

- **Release early, release often, but with a rhythm.** Raymond's essay made "release early, release often" the motto of open source. Michlmayr, Fitzgerald and Stol (IEEE Software 2015) found that volunteer projects did better once releases came on a fixed schedule: less risk per release, easier planning, and unfinished features held back instead of holding everything up.
- **Version 1.0 is a promise, not a feature count.** Semantic Versioning says that before 1.0 anything may change, and that 1.0 defines the public interface. For a server product the interface includes the upgrade path, the API, the export format and the operator's commands. Fogel's *Producing Open Source Software* advises naming the promises of a stable release in writing before it is made.
- **Projects fail for want of people, not features.** Coelho and Valente asked the maintainers of 414 failed GitHub projects why they stopped (ESEC/FSE 2017): being overtaken by a competitor, lack of time or interest of the main developer, and the project becoming obsolete led the list. Avelino et al. measured the *truck factor*, how many people must leave before a project stalls, and found it is one or two for most popular projects (ICPC 2016). A roadmap for a project with one maintainer must therefore favour work that keeps the product usable and maintainable if work pauses.
- **Scope grows by itself.** Lehman's laws of software evolution say a used system must keep changing or become less useful, and that its complexity grows unless work is spent to reduce it. Each later phase of blueprint 08 adds a second product inside the first; chapter 3 weighs each.

## 1.2 How comparable projects sequenced

| Pattern | What it means for Diffluence |
|---|---|
| A long 0.x period with a stable upgrade path before 1.0 | Diffluence already upgrades every build with the same machinery (OPS-060): a strength to keep |
| A public beta when the product can be run by strangers without help | The beta needs installation, upgrade, backup and restore to work for someone who did not build it |
| 1.0 when the interfaces stop changing and a support policy exists | The API, the export bundle format, the configuration keys and the metric names are frozen with rules for change |
| Features that many ask for beat features that are interesting to build | The enterprise document features were asked for by the product owner as basic enterprise needs (ADR-038) |

## 1.3 Where Diffluence stands

Blueprint 03 is 888 of 1,003 lines built; blueprints 04, 05 and 06 are mostly built, with what needs real servers left open; blueprint 07 is studied now. The phases of blueprint 08 after 1.0 (AI, platform, linked instances, work module, mobile, migration) are not started, which is right.

## 1.4 Recommendations

- **RMP-001** · new · **High.** Write the **beta criteria** before the beta: a stranger can install, upgrade, back up and restore with the guides alone; the HA topology ran on several machines (OPS-084); the load test met the targets on server hardware (OPS-109); no open High line in blueprints 04 to 07; signed releases with an SBOM (OPS-094). The beta is announced when they all hold.
- **RMP-002** · new · **High.** Write the **1.0 promises** as a page: what is frozen (API v1, bundle format, configuration keys, metric and log names, the `diffluence` commands), how each may change afterwards (only with a deprecation of at least one release), the support policy (OPS-098), and the upgrade promise (one command, no downtime between consecutive releases, a jump of up to six months in one step).
- **RMP-003** · new · **Medium.** Order before 1.0, after the waves already decided: (1) what makes the product safe to run for others (spec 048 waves 1 to 4), (2) the enterprise document features, in the order of chapter 2, (3) the phase 3 lines that need servers, run on the test machine, (4) the beta. The release candidate follows the beta once nothing High is open.
- **RMP-004** · new · **Medium.** Keep the truck factor in mind: every wave leaves a handoff and a report (already the way of working), every operation has a script and a guide, and nothing depends on knowledge outside the repository.
- **RMP-005** · new · **Low.** After 1.0, the phases of blueprint 08 each start with their own study and a written list of what they will *not* do (chapter 3), so none of them grows into a second product by accident.

## Sources

- Raymond: [The Cathedral and the Bazaar](http://www.catb.org/~esr/writings/cathedral-bazaar/cathedral-bazaar/), 1999.
- Michlmayr, Fitzgerald, Stol: [Why and How Should Open Source Projects Adopt Time-Based Releases?](https://doi.org/10.1109/MS.2015.55), IEEE Software 32(2), 2015.
- [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).
- Fogel: [Producing Open Source Software](https://producingoss.com/), second edition, 2017.
- Coelho, Valente: [Why Modern Open Source Projects Fail](https://doi.org/10.1145/3106237.3106246), ESEC/FSE 2017.
- Avelino, Passos, Hora, Valente: [A Novel Approach for Estimating Truck Factors](https://doi.org/10.1109/ICPC.2016.7503718), ICPC 2016.
- Lehman: [Programs, Life Cycles, and Laws of Software Evolution](https://doi.org/10.1109/PROC.1980.11805), Proceedings of the IEEE 68(9), 1980.

All sources of this study are also in [sources.md](sources.md).
