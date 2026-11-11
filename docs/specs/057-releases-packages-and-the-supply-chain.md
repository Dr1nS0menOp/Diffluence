# Spec 057: releases, packages and the supply chain

Wave O5, wave 5 of spec 048. The study is `docs/blueprint/07-operations/05-release-engineering.md`; this spec turns its lines OPS-093 to OPS-098 into a plan and records what was built. It also answers SEC-1006 (an SBOM) and SEC-1015 (signatures and provenance) of blueprint 06, and closes OPS-005 (`.deb` and `.rpm`).

## What the study asks

| Line | Asks | Priority |
|---|---|---|
| OPS-093 | A release workflow with GoReleaser: archives for Linux amd64 and arm64, `.deb` and `.rpm` with the systemd unit, checksums, and the image. Only on a tag. | High |
| OPS-094 | An SBOM in CycloneDX and SPDX for the Go module, the web packages and the image; a keyless Sigstore signature; SLSA provenance from GitHub; release notes that say how to verify. | High |
| OPS-095 | Build twice in different places and compare byte for byte; a difference fails and names the file. Binary first, then web assets, then packages. | Medium |
| OPS-096 | Base images pinned by digest, updated by Renovate. | Medium |
| OPS-097 | Named feature toggles for dark code, each with an owner and a removal issue. | Low |
| OPS-098 | The support policy in `SECURITY.md` and `release.md`. | Low |

Decisions of spec 048 that bind this wave: packages are release files with checksums and signatures, and a package repository comes later (D6); signing is keyless with Sigstore, with no long-lived key (D7); nothing phones home, and the console warns from the build date (D8, built in wave O1). The product owner approved GoReleaser in ADR-040.

## The tools

They run only in CI and in the release workflow. None of them is part of the product, and none is a Go or npm dependency.

| Tool | Version | Licence | Where |
|---|---|---|---|
| GoReleaser (OSS) | 2.20.1 | MIT | Release workflow, and the dry run in CI |
| cosign | 3.1.4 | Apache-2.0 | Release workflow: signs the checksums and the image |
| syft | 1.57.1 | Apache-2.0 | Release workflow: the SBOMs |
| `actions/attest-build-provenance` | 4.2.2 | MIT | Release workflow: SLSA provenance |
| `goreleaser/goreleaser-action`, `sigstore/cosign-installer`, `anchore/sbom-action`, `docker/login-action`, `docker/setup-qemu-action`, `docker/setup-buildx-action`, `docker/build-push-action` | Latest release on 2026-11-11 | MIT or Apache-2.0 | Release workflow |

Every action is pinned by commit SHA, with the tag in a comment, as `renovate.json` (`helpers:pinGitHubActionDigests`) keeps them. The tool versions are pinned in the workflow and the dry run.

## The plan

### 1. Packages and archives (OPS-093, OPS-005)

- `.goreleaser.yaml` builds `./cmd/diffluence` for `linux/amd64` and `linux/arm64`, with `CGO_ENABLED=0`, `-trimpath` and the same `-ldflags` as the `Makefile`: the version (`v` and the tag) and the date of the commit. `mod_timestamp` is the commit time, so files carry no build time.
- The web assets are built before GoReleaser, in the workflow (`npm ci` and `vite build`), because they are embedded in the binary.
- **Archives:** `diffluence_<version>_linux_<arch>.tar.gz` with the binary, `LICENSE`, `NOTICE`, `README.md`, `CHANGELOG.md`, the systemd unit, `install-lxc.sh` and the example configuration.
- **Packages:** `.deb` and `.rpm` through nFPM.
  - The binary goes to `/usr/bin/diffluence`, and the unit to `/usr/lib/systemd/system/diffluence.service`. The unit is made from `deploy/systemd/diffluence.service`, with only the path of the binary changed, so there is one unit to keep.
  - `postinstall` creates the system user and group `diffluence`, `/var/lib/diffluence` (0700, owned by the user) and `/etc/diffluence` (0750, `root:diffluence`). It reloads systemd and restarts the service only if it already runs (an upgrade). It never enables or starts it on a first install, because there is no configuration yet. It prints where the next steps are.
  - `preremove` stops and disables the service only when the package is really removed, not on an upgrade. The data folder, the configuration and the user are kept on removal; the guide says how to remove them.
  - The packages recommend `postgresql` and do not depend on it, since the database may run elsewhere.
- **Checksums:** `checksums.txt`, SHA-256, over every archive, package and SBOM.
- **The release** is a draft on GitHub, with the notes for the tag taken from `CHANGELOG.md` and a fixed footer on how to verify. A maintainer publishes it after reading it.

### 2. SBOM, signatures and provenance (OPS-094, SEC-1006, SEC-1015)

- **SBOMs**, each in SPDX and CycloneDX JSON:
  - one per binary (syft reads the Go module list the Go toolchain writes into it);
  - one for the source (syft reads `go.mod`, `web/package-lock.json` and `collab/package-lock.json`, so the web and collab packages that ship are listed; development-only packages are left out, as syft does by default);
  - one per image (BuildKit's own SBOM attestation, plus a syft SPDX file attached to the release).
- **Signatures:** `cosign sign-blob` signs `checksums.txt` keylessly, through the workflow's GitHub OIDC token, and writes a Sigstore bundle (`checksums.txt.sigstore.json`). Every file is covered, because every file is in `checksums.txt`. The image is signed by digest with `cosign sign`.
- **Provenance:** `actions/attest-build-provenance` attests every file in `checksums.txt`, and the image by digest, pushed to the registry. GitHub's attestations reach SLSA Build Level 2: a hosted build that signs what it built from which commit.
- **Verifying** is written in the release footer and in `install.md`: `cosign verify-blob` with the identity of the release workflow on a tag, then `sha256sum -c`; `gh attestation verify` for provenance; `cosign verify` for the image.

### 3. The release workflow

`.github/workflows/release.yml`, separate from CI:

- It runs only on a pushed tag `v<major>.<minor>.<patch>`, never on a pull request and never on a branch.
- `permissions: {}` at the top. The one job gets `contents: write` (the draft release), `packages: write` (the image on GHCR), `id-token: write` (keyless signing and attestations) and `attestations: write`.
- It refuses a tag that is not on `main`.
- The image goes to `ghcr.io/<owner>/diffluence:<version>`, for `linux/amd64` and `linux/arm64`, built from the same `Dockerfile` as CI with QEMU for arm64. That is slow, but a release is rare, and one Dockerfile stays the only recipe for the image.

### 4. Reproducibility (OPS-095)

- `scripts/reproducible.sh` copies the tree twice, to folders of a different depth and name. In each it builds the web assets and the binary, with its own Go build cache and npm cache, the same version and the same commit date. It compares every file of `internal/webui/dist/` and the binary byte for byte, names each file that differs, and exits 1 then.
- The CI job `release-dry-run` runs it, then `goreleaser check`, then a snapshot release twice, comparing the archives and packages too.
- It also installs the `.deb` in a clean Debian container and the `.rpm` in a clean Fedora container, and runs `diffluence version` there.
- The job runs on pushes to `main` and on the nightly run, not on pull requests. It only proves that a release still builds, so it need not hold up a review.

### 5. Base images by digest (OPS-096)

- The three `FROM` lines of `Dockerfile` (node, golang, distroless) carry their digest, with the tag kept for reading.
- Renovate's `config:recommended` already updates digests of Docker images it finds, so no rule is added.
- `Dockerfile.dev` stays on tags: it is a toolbox, not something shipped.
- Wave O3 adds a `backup` stage on `postgres:16`. Its digest is left to that wave or to Renovate, so the two waves don't edit the same line.

### 6. Feature toggles (OPS-097)

Not built. No feature is shipped dark today, and the coding standards say to write no code for a need that does not exist yet. `release.md` gets the rule for the first toggle instead:
- a named setting `DIFFLUENCE_FEATURE_<NAME>`, read once at start through the configuration of spec 056;
- with an owner and an issue for its removal in the changelog;
- removed within two releases.

The line becomes "partly".

### 7. The support policy (OPS-098)

`SECURITY.md` and `release.md` say:
- From 1.0, only the newest release line is supported.
- A security fix ships as a patch of that line within days.
- An installation up to six months old upgrades to the newest release in one step (spec 048 D3).

Before 1.0, nothing is supported, as today.

## How it is proven

| What | How | Where |
|---|---|---|
| The configuration is valid | `goreleaser check` | Locally in the GoReleaser container; CI dry run |
| Archives and packages build for amd64 and arm64 | `goreleaser release --snapshot --clean --skip=sign,sbom` | Locally; CI dry run |
| The `.deb` installs, creates the user and folders, places the unit, and the service starts against a test database | Install in a clean Debian 12 container with systemd as PID 1 and a PostgreSQL container beside it; `systemctl start diffluence`; `/readyz` | Locally |
| The `.rpm` installs and runs | `rpm -i` in a clean Fedora container, `diffluence version` | Locally; CI dry run |
| The binary and the web assets reproduce | `scripts/reproducible.sh` | Locally; CI dry run |
| The packages reproduce | Two snapshot runs, `checksums.txt` compared | Locally; CI dry run |
| The SBOMs list Go modules and npm packages | syft on the binary and the source tree | Locally, with the syft container |
| The image builds with the pinned digests | `docker build` under a throw-away tag | Locally |
| Signing, attestations, the image push, the draft release | Only a pushed tag on GitHub can run them | Not before the first tag |

## What only a tag can prove

- That GitHub gives the workflow an OIDC token, and that Sigstore's Fulcio and Rekor accept it.
- That the attestations appear on the release files and on the image.
- That the image reaches GHCR for both architectures.
- That the draft release has the right notes and footer.

The first tag, a release candidate such as `v0.1.0-rc.1`, does not match the workflow's pattern. So the first real check is the first `v0.x.y` tag. The checklist in `release.md` says to verify each signature by hand after that run.

## Load budget (ARC-238)

None. Nothing here runs in the product. The dry run adds a CI job of about eight minutes on `main`, beside the others.

## Decided in this wave

- **D1. The image is built by the existing `Dockerfile` with buildx and QEMU, not by GoReleaser.** One recipe for the image keeps CI and the release image the same; arm64 under QEMU is slow, but releases are rare.
- **D2. One systemd unit, two paths.** The package unit is made from `deploy/systemd/diffluence.service` at release time, with `/usr/local/bin` changed to `/usr/bin`, because files installed by a package belong under `/usr` and files installed by hand under `/usr/local`.
- **D3. A package never starts the service on a first install,** because it has no database address yet. It does restart a running service on an upgrade, as Debian packages do, and the checks before an upgrade (spec 054) then decide whether it may migrate.
- **D4. The release is a draft.** A person reads the notes and publishes it; the image is pushed before that, under the version tag only.
- **D5. The dry run runs on `main` and nightly, not on pull requests,** following the product owner's wish for quicker checks during development (2026-11-11).
- **D6. No toggle code** (section 6).
- **D7. The systemd unit sets `DIFFLUENCE_DATA_DIR` and `DIFFLUENCE_HTTP_ADDR` itself.** Installing the package in a Debian container with systemd showed that the unit worked only when the environment file named the data folder, as `install-lxc.sh` writes it: otherwise the embedded queue tried to keep its store outside the one folder the sandbox lets the service write, and the service restarted again and again. The unit now carries both defaults, and the environment file may still change them. With them, the full sandbox of the unit starts and `/readyz` answers 200; no sandbox option had to be loosened.
- **D8. Two kinds of file differ between builds by design:** the SBOMs, because syft writes a creation time and a document id. The reproducibility check compares the binary, the web assets, the archives and the packages, not the SBOMs.

## Found while building

- An RPM records the name of the machine that built it, so two builds differed in that one field. The configuration now sets a fixed build host (`diffluence-release`), and the `.rpm` files reproduce.
- GoReleaser runs syft inside `dist/`, so the SBOM of the source scans the folder above it (`dir:..`), and syft's exclude patterns need `/**`.
