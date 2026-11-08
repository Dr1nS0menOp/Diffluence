# Security risks of using Diffluence

Read this before you put real information in Diffluence. The first part is for everyone. After that come a part for people who run it and a part for people who write and read in it.

Last updated: 6 November 2026. Diffluence is **alpha** software.

## The short version

- **There is no guarantee.** Diffluence comes as it is, without warranty. It may contain security flaws, known or unknown, and they can lead to data being lost, changed or read by the wrong people.
- **Security updates are not guaranteed.** Nobody promises that the software is actively kept up to date, that a flaw is fixed within a set time, or that this project still exists next year. There is no support contract and no help desk.
- **It has not been independently tested.** No outside company has done a penetration test or an audit, and Diffluence holds no certification. All testing so far was done by the project itself.
- **You carry the risk.** Whoever installs Diffluence is responsible for the server, the backups, the updates and the data, and for meeting the law that applies to that data.

If you need guarantees, a support contract or a certified product, Diffluence is not that today.

## What "no guarantee" means

Diffluence is licensed under the [Apache License 2.0](../../LICENSE). Section 7 says the software is provided "AS IS", without warranties of any kind. Section 8 says the contributors are not liable for damages from using it.

The response times in [SECURITY.md](../../SECURITY.md) are aims, not promises. Before 1.0, no version is supported at all.

## How it is built and tested

You should know this to judge the risk yourself.

- **Automated checks, not human review.** Much of the code is written with an AI assistant. No person reviews every line. Automated tests stand in for that review, including tests that break the permission code on purpose to prove that the tests notice ([mutation testing](#research)). Research found that people using an AI assistant wrote less secure code in a controlled study ([Perry et al.](#research)). Checks reduce that risk. They do not remove it.
- **Tests only find what they look for.** Passing tests show that known mistakes are absent. They cannot show that no mistakes exist.
- **The project attacks its own software.** Such a test has a blind spot: whoever wrote the software misses the same things twice. It is no replacement for an independent penetration test. What it measured is in [the limits](../operations/limits.md).
- **Some parts were tested against stand-ins.** The optional helpers for PDF, office files and diagrams have not all been run for real. The Helm chart has not run on a real cluster. Tests with very many pages and with many people editing one page are not finished.

## What can go wrong

| Risk | What it would mean |
|---|---|
| A flaw in the permission code | Someone reads or changes a page they should not reach |
| Script running in a page | An attacker acts in the browser of a person who opens the page |
| A stolen account or session | An attacker works as that person until the session ends |
| A malicious file or import | A file harms a helper service, or uses up memory or disk |
| A compromised dependency | Code from a library Diffluence relies on does something harmful ([Ladisa et al.](#research)) |
| A server that is not kept up to date | A known, published flaw is used against your installation |
| Lost or broken backups | Data is gone for good |
| Too much load | The service slows down or stops |

Diffluence has measures against each of these, listed in the [threat model](threat-model.md). A measure lowers a risk. It does not end it.

## For people who run Diffluence

**Do not rely on these, because they are not there or not finished:**

- **Page text is not encrypted by Diffluence at rest.** Search needs the text. Stored files are encrypted, but the database is not: use disk encryption on the server.
- **Without `master.key`, no file can be read.** Files are encrypted with keys that `master.key` protects. Back it up apart from the data, and do not lose it.
- **Files are served from the same address as the app.** Other measures cover the known attacks, but a separate address would be stronger.
- **Limits on login attempts are counted per node.** A restart clears them, and two nodes count apart.
- **Optional helpers** run as separate programs, each with its own flaws and updates. Only turn on what you need.

**What you should do:**

1. **Keep it off the open internet** where you can: use a VPN or an internal network. If it must be reachable, put it behind HTTPS.
2. **Watch for updates yourself.** Follow the releases and security advisories of this repository, and update soon after a fix. Nobody will warn you personally.
3. **Update the rest too:** the operating system, Docker, PostgreSQL and every helper.
4. **Make backups, and test restoring them.** Keep the backup key apart from the backups. Without it, a backup cannot be read. See [backup and restore](../operations/backup-restore.md).
5. **Log in through an identity provider or with passkeys**, certainly for administrators. Keep the number of administrators small.
6. **Keep secrets in files or environment variables**, as the install guide says, and limit who can reach the server. Anyone with access to the server or the database can get around everything in the app.
7. **Check the audit log.** Run `diffluence audit verify`, and keep a copy of the log elsewhere.
8. **Limit resources**, so one heavy task cannot stop the whole server.
9. **Try it with data you can lose** before you trust it with data you cannot.

**The law.** The organisation that runs an installation decides what happens with the data, and is usually the controller under the GDPR. It has to assess whether Diffluence is suitable for that data. This document is not legal advice.

## For people who write and read in Diffluence

- **Your administrator is in charge.** The people who run the server can reach what is stored on it, also pages that are restricted in the app. Actions are recorded in an audit log, and they may see what you did.
- **Do not store passwords, keys or other secrets in pages.** Pages keep their old versions, so deleting the text later does not remove it from the history. Backups keep it longer still.
- **Deleted does not mean gone at once.** Pages stay in the trash for a while, and in backups after that.
- **Check who can see a page** before you write something sensitive. A space can be open to more people than you think, and a restriction can be changed by those who manage it.
- **Think before sharing.** Whoever has a public link can read the page. Whatever people download or export is out of reach of Diffluence.
- **Be careful with attachments.** A file from a colleague can still be harmful. Open it as you would an attachment in an e-mail.
- **Watch out for false messages.** A mail that asks you to log in may not come from Diffluence. Check the address, and go to the site yourself.
- **Protect your account.** Use a passkey, or an authenticator app. Log out on a shared computer.
- **Report anything odd** to your administrator: a page you should not see, a login you do not recognise.

## Research

During the build, the project studied current security standards and wrote down, line by line, how Diffluence relates to them. That work is in [blueprint 06](../blueprint/06-security-and-compliance.md), with its [sources](../blueprint/06-security-and-compliance/sources.md).

**Standards:**

- [OWASP Application Security Verification Standard 5.0](https://asvs.owasp.org/) and [OWASP Top 10:2025](https://top10.owasp.org/2025)
- [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html), on login and passwords
- [W3C Web Authentication Level 3](https://www.w3.org/TR/webauthn-3/) and [Content Security Policy Level 3](https://www.w3.org/TR/CSP3/)
- [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700), best practice for OAuth 2.0 security
- [SLSA 1.1](https://slsa.dev/spec/v1.1/levels), on the supply chain

**Academic background:** published research on the kinds of measures Diffluence uses.

| Topic | Publication |
|---|---|
| Design principles, such as deny by default and least privilege | J. H. Saltzer and M. D. Schroeder, [The protection of information in computer systems](https://doi.org/10.1109/PROC.1975.9939), Proceedings of the IEEE, 1975 |
| Storing passwords (Argon2id) | A. Biryukov, D. Dinu and D. Khovratovich, [Argon2: New Generation of Memory-Hard Functions for Password Hashing and Other Applications](https://doi.org/10.1109/EuroSP.2016.31), IEEE EuroS&P, 2016 |
| Login without passwords | J. Bonneau, C. Herley, P. C. van Oorschot and F. Stajano, [The Quest to Replace Passwords](https://doi.org/10.1109/SP.2012.44), IEEE Symposium on Security and Privacy, 2012 |
| Audit logs that show tampering | B. Schneier and J. Kelsey, [Secure audit logs to support computer forensics](https://doi.org/10.1145/317087.317089), ACM Transactions on Information and System Security, 1999 |
| Blocking scripts in the browser | L. Weichselbaum, M. Spagnuolo, S. Lekies and A. Janc, [CSP Is Dead, Long Live CSP!](https://doi.org/10.1145/2976749.2978363), ACM CCS, 2016 |
| Cross-site requests | A. Barth, C. Jackson and J. C. Mitchell, [Robust defenses for cross-site request forgery](https://doi.org/10.1145/1455770.1455782), ACM CCS, 2008 |
| Attacks through dependencies | P. Ladisa, H. Plate, M. Martinez and O. Barais, [SoK: Taxonomy of Attacks on Open-Source Software Supply Chains](https://doi.org/10.1109/SP46215.2023.10179304), IEEE Symposium on Security and Privacy, 2023 |
| Testing the tests | Y. Jia and M. Harman, [An Analysis and Survey of the Development of Mutation Testing](https://doi.org/10.1109/TSE.2010.62), IEEE Transactions on Software Engineering, 2011 |
| Security of code written with AI | N. Perry, M. Srivastava, D. Kumar and D. Boneh, [Do Users Write More Insecure Code with AI Assistants?](https://doi.org/10.1145/3576915.3623157), ACM CCS, 2023 |

Following a standard or a paper does not make software secure. These sources show what good practice looks like, not that Diffluence meets it without error.

## Found a problem?

Do not open a public issue. See [SECURITY.md](../../SECURITY.md).
