# Security policy

## Supported versions

Diffluence is **alpha**. No version is supported before 1.0. From 1.0 on:

- only the newest release line receives fixes;
- a security fix ships as a patch of that line within days of its confirmation;
- an installation up to six months old upgrades to the newest release in one step.

Releases are signed with Sigstore and carry SLSA provenance and an SBOM; `docs/operations/install.md` says how to check them.

## Reporting a vulnerability

**Do not open a public issue.** Use GitHub's *Report a vulnerability* button (private vulnerability reporting) on this repository. If you cannot use it, mail DiffluenceCreator@protonmail.com.

Please include the affected version or commit, the steps to reproduce, and the impact you expect.

## What to expect

These are aims, not promises. Diffluence comes without guarantee, and security updates are not guaranteed. See [the security risks](docs/security/risks.md).

- Acknowledgement within 3 working days
- A first assessment within 7 days
- Critical issues fixed in a release within 7 days of confirmation
- Credit in the advisory, unless you prefer not to be named
- A CVE assigned through GitHub where appropriate

## Scope

In scope: this repository's code, the official container images and the release artefacts. Out of scope: vulnerabilities in your own infrastructure, social engineering, and denial of service through volume alone.
