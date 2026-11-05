# 06: Sources

Read on 2026-11-05 for this study. The code was read on `main` at commit `2970b0f`, with the plans in `docs/specs/010` to `015`. The main findings were checked again on branch `wave-f` at `a017fdc`. Long documents were read for the parts cited, not from cover to cover. Everything in these chapters is in our own words.

## Standards and specifications

| Source | Used for |
|---|---|
| OWASP ASVS 5.0.0 (30 May 2025): <https://asvs.owasp.org/>, chapters at <https://github.com/OWASP/ASVS/tree/v5.0.0/5.0/en> | Chapter 12, levels and every ASVS reference |
| OWASP Top 10:2025: <https://top10.owasp.org/2025> | Chapter 12 |
| NIST SP 800-63B-4: <https://pages.nist.gov/800-63-4/sp800-63b.html> | Chapter 2 |
| RFC 9700, OAuth 2.0 security best practice: <https://www.rfc-editor.org/rfc/rfc9700> | Chapter 2 |
| RFC 7644, SCIM protocol, section 7: <https://www.rfc-editor.org/rfc/rfc7644> | Chapter 2 |
| W3C Web Authentication Level 3: <https://www.w3.org/TR/webauthn-3/> | Chapter 2 |
| OpenID Connect Back-Channel Logout 1.0: <https://openid.net/specs/openid-connect-backchannel-1_0.html> | Chapter 2 |
| RFC 8058, one-click unsubscribe: <https://www.rfc-editor.org/rfc/rfc8058> | Chapter 5 |
| W3C Web Annotation Data Model: <https://www.w3.org/TR/annotation-model/> | Chapter 5 |
| Standard Webhooks: <https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md> | Chapter 5 |
| W3C Content Security Policy Level 3: <https://www.w3.org/TR/CSP3/> | Chapter 6 |
| WHATWG HTML, the `iframe` element: <https://html.spec.whatwg.org/multipage/iframe-embed-object.html> | Chapter 6 |
| RFC 9116, `security.txt`: <https://www.rfc-editor.org/rfc/rfc9116> | Chapter 11 |
| SLSA 1.1, build levels: <https://slsa.dev/spec/v1.1/levels> | Chapter 11 |

## OWASP cheat sheets

From <https://cheatsheetseries.owasp.org/>: Server-Side Request Forgery Prevention, File Upload, Logging, Forgot Password, Session Management.

## Law

**This is not legal advice.** These texts were read by a non-lawyer, and no lawyer has checked what the chapters say about them.

| Source | Used for |
|---|---|
| GDPR articles 5, 6, 9, 12, 15, 17, 20, 25, 28, 30, 32, 33, 35, 88 and recital 78: <https://gdpr-info.eu/> | Chapter 10 |
| Wet op de ondernemingsraden, article 27(1), points k and l: <https://wetten.overheid.nl/BWBR0002747/> | Chapter 10 |
| NIS2 Directive article 21: <https://www.nis-2-directive.com/NIS_2_Directive_Article_21.html>, because the EUR-Lex page came back empty | Chapter 12 |
| Cyberbeveiligingswet, in force 15 August 2026: <https://www.forvismazars.com/nl/nl/wie-zijn-wij/nieuws-events-en-publicaties/nieuws/cyberbeveiligingswet-vanaf-15-augustus-2026> | Chapter 11 |
| Cyber Resilience Act: <https://digital-strategy.ec.europa.eu/en/policies/cyber-resilience-act> | Chapter 11 |
| OpenSSF on open-source stewards: <https://policy.openssf.org/CRA/stewards-one-pager.html> | Chapter 11 |
| A Goodwin alert on the CRA, and a noze.it article on the Commission's guidance of 27 July 2026. Their addresses were not kept, so find them again before anyone relies on them | Chapter 11 |
| BIO2: <https://www.nldigitalgovernment.nl/overview/government-information-security-baseline/> and <https://zoek.officielebekendmakingen.nl/stcrt-2026-7416.pdf> | Chapter 12 |

## Products

| Source | Used for |
|---|---|
| go-webauthn, a dependency we use: <https://pkg.go.dev/github.com/go-webauthn/webauthn/webauthn>, and `webauthn/types.go` and `login.go` from its repository | SEC-134 |
| Go `CrossOriginProtection`: <https://pkg.go.dev/net/http#CrossOriginProtection> | Chapter 3 |
| Go `net/smtp`: <https://pkg.go.dev/net/smtp> | Chapter 5 |
| Mailpit: <https://pkg.go.dev/github.com/axllent/mailpit-test> | Chapter 5 |
| Gotenberg configuration: <https://gotenberg.dev/docs/configuration> | Chapter 6 |
| CVE-2026-42595 in Gotenberg: <https://osv.dev/vulnerability/GHSA-chwh-f6gm-r836> | Chapter 6 |
| draw.io embed mode: <https://www.drawio.com/docs/reference/embed-mode/> and <https://github.com/jgraph/drawio/wiki/Embedding> | Chapter 6 |
| WOPI proof keys: <https://learn.microsoft.com/en-us/microsoft-365/cloud-storage-partner-program/online/scenarios/proofkeys> | Chapter 6 |
| Hocuspocus hooks: <https://tiptap.dev/docs/hocuspocus/server/hooks> | Chapter 6 |
| PdfCraft: <https://getartcraft.com/apps/pdfcraft> and <https://github.com/storytold/pdfcraft> | Chapter 6 |
| Euro-Office: <https://heise.de/-11322160> | Chapter 6 |
| SecLists licence: <https://raw.githubusercontent.com/danielmiessler/SecLists/master/LICENSE> | Chapter 2 |

## Not read

- **Collabora Online's documentation** sits behind a bot check, and its configuration file was not at the expected address. See SEC-532.
- **Hocuspocus's page on authentication** answered 404. The page on hooks was used.
- **The ISO/IEC 27002 text** is paid for, so chapter 12 stays at the level of themes.
- **The Commission's guidance on the Cyber Resilience Act of 27 July 2026**, and Annex I of the Act.
- **ASVS V9 and the server sections of V10** were not walked line by line, because we issue no such tokens and are no authorization server.
