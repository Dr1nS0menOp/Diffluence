# 07.5: Release engineering: versions, support, packages and the supply chain

How a release is numbered, how long it is looked after, how it reaches a machine, and how an operator can trust that it is what the project built. Blueprint 06 (chapter 11) holds the security lines for the supply chain; this chapter adds the operational side and the research behind both.

## 5.1 Versions and the rhythm of releases

- **Semantic Versioning 2.0.0** gives `MAJOR.MINOR.PATCH` a meaning: a major breaks, a minor adds, a patch fixes. Before 1.0 anything may change. For a server application, "breaks" means what an operator must do, not only the API.
- **Time-based releases work for volunteer projects.** Michlmayr, Fitzgerald and Stol interviewed members of seven large volunteer projects that moved from "release when ready" to a fixed rhythm (IEEE Software 2015). The fixed date helped planning, made releases smaller and less risky, and stopped one unfinished feature from holding up everything else. The conditions: a short freeze before each release, and features that can be held back (switched off) when they are not ready. Diffluence's monthly plan after 1.0 fits this.
- **Feature toggles let unfinished work ship dark.** Rahman, Querel, Rigby and Adams (MSR 2016) studied toggles in practice: they make a fixed rhythm possible, and they turn into debt when they are never removed. The common rule is that each toggle has an owner and a date to remove it.

## 5.2 How long a release is looked after

| Project | Support |
|---|---|
| PostgreSQL | Each major version for five years |
| Kubernetes | The three newest minor versions, about 14 months each |
| GitLab | Monthly releases; security fixes for the current and the two previous monthly releases |
| Node.js | Even majors become LTS, supported for 30 months |
| Nextcloud | Each major about a year, with paid extended support |

A project without staff cannot back-port fixes to many lines. Blueprint 07 therefore supports only the newest line. The cost lands on the operator, who must upgrade monthly. The industry answer that keeps that bearable is the one Diffluence already has: an upgrade that is one command, never needs a person, and can be undone (chapter 1). One extra promise makes it fairer: how far back an installation may be and still jump straight to the newest release (decided in spec 048: six months).

## 5.3 Packages

GoReleaser builds release archives for many platforms and, through its nFPM part, `.deb` and `.rpm` packages with a systemd unit, a user, and the folders in place, without needing Debian or Red Hat tools. Most Go server projects that ship packages use it. Packages can be attached to the release as files with checksums; a signed `apt` and `yum` repository is a later step that needs hosting.

## 5.4 The supply chain

- **The threat.** Ladisa et al. mapped the attacks on open-source supply chains (IEEE S&P 2023): taking over a maintainer's account, poisoning a build, publishing a look-alike package, or changing what a dependency does in a new version. A wiki that holds a company's knowledge is worth such an attack.
- **Signing that people actually use.** Sigstore (Newman, Meyers and Torres-Arias, ACM CCS 2022) signs with a short-lived certificate bound to an identity, such as the CI workflow of a repository, and writes every signature in a public log. No long-lived key to lose. `cosign` is its tool, and GitHub can produce such signatures for build provenance.
- **Provenance.** SLSA 1.0 defines build levels. Level 2 asks for a hosted build service that signs a statement of what it built from which source; GitHub's artifact attestations reach it with little work. Level 3 asks that the build cannot be influenced by the project's own steps. in-toto (Torres-Arias et al., USENIX Security 2019) is the format underneath.
- **Reproducible builds.** Lamb and Zacchiroli (IEEE Software 2022) argue that only a build anyone can repeat bit for bit lets others check a binary against its source. Go has produced perfectly reproducible toolchains since 1.21, and Go programs built with `-trimpath`, without cgo and with a fixed version reproduce well. The web assets built by Vite need the same care: a locked `package-lock.json` and no timestamps.
- **SBOM.** A list of every component, in SPDX or CycloneDX. The NTIA minimum elements (2021) say what it must hold. The EU Cyber Resilience Act will expect one for products placed on the market, and customers already ask.

## 5.5 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| Version from the tag, `diffluence version` | Built | `Makefile`, `internal/version/version.go` |
| `-trimpath`, no cgo, stripped | Built | `Makefile`, `Dockerfile` |
| Release checklist with an upgrade-impact line | Built | `docs/operations/release.md`, OPS-062 |
| GitHub Actions pinned to commit SHAs | Built | `renovate.json`, `.github/workflows/ci.yml` |
| Base images pinned by digest | Partly: Gotenberg is, the build and runtime images use tags | `Dockerfile`, `compose.yaml` |
| `.deb` and `.rpm` | Not built | OPS-005 |
| SBOM, signatures, provenance | Not built | SEC-1006, SEC-1015 |
| A check that a build reproduces | Not built | |
| Feature toggles for dark code | Not built | OPS-039 |

## 5.6 Recommendations

- **OPS-093** · built (spec 057; the release workflow first runs on the first tag) · **High.** A release workflow with GoReleaser: archives for Linux amd64 and arm64, `.deb` and `.rpm` through nFPM with the systemd unit of `deploy/systemd/`, checksums, and the container image. Approved by the product owner on 2026-11-10. It runs only on a tag. This closes OPS-005 and is the base of OPS-032.
- **OPS-094** · partly (spec 057: SBOMs built and proven; signing and provenance written, proven only by the first tagged release) · **High.** Each release gets an SBOM in CycloneDX and SPDX for the Go module, the web packages and the image, a keyless Sigstore signature, and a SLSA provenance attestation from GitHub. The release notes say how to verify them. This answers SEC-1006 and SEC-1015.
- **OPS-095** · built (spec 057: binary, web assets, archives and `.deb` and `.rpm`; SBOMs carry a time and an id by design) · **Medium.** CI builds the release twice in different places and compares the results byte for byte. A difference fails the build and names the file. Start with the Go binary, then the web assets, then the packages.
- **OPS-096** · partly (spec 057: the three base images of `Dockerfile`; the images of `compose.yaml` and the backup stage still use tags) · **Medium.** Pin the build and runtime base images by digest and let Renovate update the digests, as for actions.
- **OPS-097** · partly (spec 057: the rule is in `release.md`; no feature ships dark, so no toggle exists) · **Low.** Dark features: a small list of named toggles in configuration (chapter 6), each with an owner and an issue for its removal, read once at start. No toggle service and no dependency.
- **OPS-098** · built (spec 057) · **Low.** Write the support policy in `SECURITY.md` and `release.md`: the newest line is supported, a security fix ships as a patch of that line within days, and an installation up to six months old upgrades in one step.

## Sources

- [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).
- Michlmayr, Fitzgerald, Stol: [Why and How Should Open Source Projects Adopt Time-Based Releases?](https://doi.org/10.1109/MS.2015.55), IEEE Software 32(2), 2015.
- Rahman, Querel, Rigby, Adams: [Feature Toggles: Practitioner Practices and a Case Study](https://doi.org/10.1145/2901739.2901745), MSR 2016.
- Ladisa, Plate, Martinez, Barais: [SoK: Taxonomy of Attacks on Open-Source Software Supply Chains](https://doi.org/10.1109/SP46215.2023.10179304), IEEE S&P 2023.
- Newman, Meyers, Torres-Arias: [Sigstore: Software Signing for Everybody](https://doi.org/10.1145/3548606.3560596), ACM CCS 2022.
- Torres-Arias, Afzali, Kuppusamy, Curtmola, Cappos: [in-toto: Providing Farm-to-Table Guarantees for Bits and Bytes](https://www.usenix.org/conference/usenixsecurity19/presentation/torres-arias), USENIX Security 2019.
- Lamb, Zacchiroli: [Reproducible Builds: Increasing the Integrity of Software Supply Chains](https://arxiv.org/abs/2104.06020), IEEE Software 39(2), 2022.
- Cox: [Perfectly Reproducible, Verified Go Toolchains](https://go.dev/blog/rebuild), The Go Blog, 2023.
- [SLSA 1.0, security levels](https://slsa.dev/spec/v1.0/levels).
- NTIA: [The Minimum Elements For a Software Bill of Materials](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom), 2021.
- [CycloneDX](https://cyclonedx.org/specification/overview/), [SPDX](https://spdx.dev/).
- GitHub: [Artifact attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds).
- GoReleaser: [nFPM, Linux packages](https://goreleaser.com/customization/nfpm/).
- Support policies: [PostgreSQL versioning](https://www.postgresql.org/support/versioning/), [Kubernetes releases](https://kubernetes.io/releases/), [GitLab maintenance policy](https://docs.gitlab.com/policy/maintenance/), [Node.js releases](https://nodejs.org/en/about/previous-releases), [Nextcloud maintenance and release schedule](https://github.com/nextcloud/server/wiki/Maintenance-and-Release-Schedule).

All sources of this study are also in [sources.md](sources.md).
