# 06.12: How this maps to standards

Customers and auditors ask in the words of standards, so this chapter answers in those words. Requirement numbers are from **OWASP ASVS 5.0.0**, cited in full as `v5.0.0-6.2.1`. The text in each row is our own short version, and levels were checked against the requirement tables. A state is this study's own reading of the code. It is not an audit.

| Word | Means |
|---|---|
| **ok** | Met today |
| **part** | Partly met. The SEC line says what is missing |
| **plan** | In a wave or phase |
| **gap** | Open, and in no plan yet |
| **op** | The operator's part, which the guide must cover |
| **later** | Chosen to wait. The SEC line or the row says why |
| **n/a** | Does not apply to Diffluence |

## 12.1 ASVS 5.0.0

The level column is the ASVS level. Level 2 is our target (SEC-001).

### V1 Encoding and sanitization

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 1.1.1 | 2 | Decode once, before use | ok | SEC-223 |
| 1.1.2 | 2 | Encode at the last step | ok | SEC-258 |
| 1.2.1 | 1 | Encode by context | ok | SEC-258, SEC-417 |
| 1.2.2 | 1 | Safe URL protocols only | ok | SEC-315 |
| 1.2.3 | 1 | Safe JSON building | ok | Standard encoder |
| 1.2.4 | 1 | Parameterised queries | ok | SEC-220 |
| 1.2.5 | 1 | No OS command injection | ok | The server runs no commands |
| 1.2.6 to 1.2.8 | 2 | LDAP, XPath, LaTeX | n/a | |
| 1.2.9 | 2 | Escape regular expressions | ok | No regular expression is built from input |
| 1.2.10 | 3 | CSV and formula injection | ok | SEC-361, SEC-362 |
| 1.3.1 | 1 | Sanitise editor output | ok | SEC-300 |
| 1.3.2 | 1 | No `eval` | ok | SEC-250 |
| 1.3.3 | 2 | Sanitise for dangerous contexts | ok | SEC-301 |
| 1.3.4 | 2 | SVG | ok | SEC-325 |
| 1.3.5 | 2 | Markdown and other scriptable content | ok | SEC-300 |
| 1.3.6 | 2 | SSRF allow-lists | plan | SEC-431 |
| 1.3.7 | 2 | No template injection | ok | No template is built from input |
| 1.3.8, 1.3.9 | 2 | JNDI, memcache | n/a | |
| 1.3.10 | 2 | Format strings | ok | Go, fixed formats |
| 1.3.11 | 2 | SMTP injection | plan | SEC-417 |
| 1.3.12 | 3 | ReDoS | ok | Go's engine has no backtracking, and browser patterns are fixed |
| 1.4.1 to 1.4.3 | 2 | Memory safety | n/a | The section is for systems languages and unmanaged code |
| 1.5.1 | 1 | XML parsers | plan | SEC-349 |
| 1.5.2 | 2 | Safe deserialisation | ok | JSON into typed structs |
| 1.5.3 | 3 | Parsers agree | part | One gate for all ways in, SEC-305 |

### V2 Validation and business logic

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 2.1.1 to 2.1.3 | 1, 2 | Document the rules | part | Specs hold them, per feature |
| 2.2.1 to 2.2.3 | 1, 2 | Validate on the server | ok | SEC-011, SEC-301 |
| 2.3.1 | 1 | Steps in order | ok | SEC-122 |
| 2.3.2 | 2 | Limits are enforced | part | SEC-231, SEC-273 |
| 2.3.3 | 2 | Atomic operations | ok | SEC-800 |
| 2.3.4 | 2 | Locks | ok | Tree and page locks |
| 2.3.5 | 3 | Two people for high-value actions | later | Level 3 |
| 2.4.1 | 2 | Anti-automation | gap | SEC-231 |
| 2.4.2 | 3 | Human timing | n/a | |

### V3 Web frontend

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 3.1.1 | 3 | Document browser needs | part | Chapter 3 |
| 3.2.1 | 1 | No rendering in the wrong context | ok | SEC-325, SEC-326 |
| 3.2.2 | 1 | Safe DOM functions | ok | SEC-258 |
| 3.2.3 | 3 | DOM clobbering | part | SEC-320 |
| 3.3.1 to 3.3.4 | 1, 2 | Cookie attributes | ok | SEC-171, SEC-153 |
| 3.3.5 | 3 | Cookie size | ok | |
| 3.4.1 | 1 | HSTS | part | SEC-256, SEC-257 |
| 3.4.2 | 1 | CORS | ok | SEC-225 |
| 3.4.3 | 2 | CSP | ok | SEC-250 |
| 3.4.4 | 2 | `nosniff` | ok | SEC-253 |
| 3.4.5 | 2 | Referrer policy | ok | SEC-253 |
| 3.4.6 | 2 | `frame-ancestors` | ok | SEC-250 |
| 3.4.7 | 3 | CSP reports | part | SEC-255 |
| 3.4.8 | 3 | COOP | ok | SEC-253 |
| 3.5.1 | 1 | Cross-origin checks | ok | SEC-240 |
| 3.5.2 | 1 | Preflight | n/a | SEC-240 is used instead |
| 3.5.3 | 1 | Unsafe methods for changes | ok | SEC-240, SEC-242 |
| 3.5.4 | 2 | Separate hostnames | part | SEC-331, SEC-521, SEC-555 |
| 3.5.5 | 2 | `postMessage` origins | gap | SEC-523, SEC-557 |
| 3.5.6 to 3.5.8 | 3 | JSONP, script data, cross-origin loads | ok | SEC-253 |
| 3.6.1 | 3 | Outside assets | ok | SEC-900 |
| 3.7.1 | 2 | Supported technology | ok | |
| 3.7.2 | 2 | Redirects | ok | SEC-144 |
| 3.7.3 | 3 | Warn before leaving | later | SEC-318 |
| 3.7.4 | 3 | HSTS preload | op | |
| 3.7.5 | 3 | Missing browser features | part | SEC-240 explains the fallback |

### V4 API and web service

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 4.1.1 | 1 | Content types | ok | |
| 4.1.2 | 2 | Redirects to https | part | Left to the proxy |
| 4.1.3 | 2 | Headers from intermediaries | ok | SEC-012 |
| 4.1.4 | 3 | Only known methods | ok | SEC-226 |
| 4.1.5 | 3 | Message signatures | n/a | Webhooks are signed all the same, SEC-440 |
| 4.2.1 to 4.2.5 | 2, 3 | HTTP framing | ok | Go's server, and the proxy |
| 4.3.1, 4.3.2 | 2 | GraphQL | n/a | |
| 4.4.1 to 4.4.4 | 1, 2 | WebSockets | plan | SEC-540, SEC-541 |

### V5 File handling

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 5.1.1 | 2 | Document file rules | part | Chapter 4 |
| 5.2.1 | 1 | Size limits | ok | SEC-270 |
| 5.2.2 | 1 | Content matches type | ok | SEC-325 |
| 5.2.3 | 2 | Limits on archives | ok | SEC-345 |
| 5.2.4 | 3 | Quotas | gap | SEC-273 |
| 5.2.5 | 3 | Symbolic links | ok | SEC-346 |
| 5.2.6 | 3 | Pixel limits | n/a | The server never decodes an image, SEC-335 |
| 5.3.1, 5.3.2 | 1 | No execution, safe paths | ok | SEC-327 |
| 5.3.3 | 3 | Paths from archives | ok | SEC-346 |
| 5.4.1, 5.4.2 | 2 | Names in headers | ok | SEC-328 |
| 5.4.3 | 2 | Antivirus | later | SEC-333. Decision for the product owner |

### V6 Authentication

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 6.1.1 | 1 | Document rate limits and lockout | gap | SEC-111 |
| 6.1.2 | 2 | Context words | gap | SEC-106 |
| 6.1.3 | 2 | Document pathways | part | Chapter 2 starts this |
| 6.2.1 | 1 | Length | plan | SEC-102 |
| 6.2.2, 6.2.3 | 1 | Changing a password | ok | SEC-115 |
| 6.2.4 | 1 | Common passwords | plan | SEC-105 |
| 6.2.5 to 6.2.8 | 1 | No composition, masked, paste, exact | ok | SEC-104, SEC-114 |
| 6.2.9, 6.2.10 | 2 | Long, no rotation | ok | SEC-103, SEC-104 |
| 6.2.11 | 2 | Context words used | gap | SEC-106 |
| 6.2.12 | 2 | Breached passwords | plan | SEC-105 |
| 6.3.1 | 1 | Stuffing and brute force | part | SEC-109 |
| 6.3.2 | 1 | No default accounts | ok | SEC-185 |
| 6.3.3 | 2 | Multi-factor | plan | SEC-160, SEC-163 |
| 6.3.4 | 2 | No hidden pathways | ok | SEC-133 |
| 6.3.5, 6.3.7 | 3 | Tell people | plan | ACC-017, SEC-127 |
| 6.3.6 | 3 | No mail as a factor | ok | |
| 6.3.8 | 3 | No enumeration | ok | SEC-108, SEC-187 |
| 6.4.1 | 1 | Initial passwords | part | SEC-116 |
| 6.4.2 | 1 | No hints | ok | |
| 6.4.3, 6.4.4 | 2 | Reset keeps the factor, lost factors | plan | SEC-187, SEC-188 |
| 6.4.5 | 3 | Renewal notices | n/a | |
| 6.4.6 | 3 | Admins cannot set a password | part | SEC-116 |
| 6.5.1 | 2 | Single use | ok | SEC-121 |
| 6.5.2 | 2 | Salted hash under 112 bits | gap | SEC-124 |
| 6.5.3 to 6.5.5 | 2 | Random, entropy, lifetime | ok | SEC-700, SEC-121 |
| 6.5.6 | 3 | Revoke any factor | ok | |
| 6.5.7 | 3 | Biometrics | n/a | |
| 6.5.8 | 3 | Server time | ok | SEC-121 |
| 6.6.1 to 6.6.4 | 2, 3 | SMS and push | n/a | |
| 6.7.1 | 3 | Certificates | n/a | |
| 6.7.2 | 3 | Challenge size | ok | SEC-132 |
| 6.8.1, 6.8.2 | 2 | Provider identity, signatures | ok | SEC-142, SEC-143 |
| 6.8.3 | 2 | SAML replay | n/a | |
| 6.8.4 | 2 | Strength from the provider | plan | SEC-150 |

ASVS V6 has no requirement on user verification for passkeys, so SEC-134 appears in no row above. It is still one of the most serious open lines for login.

### V7 Session management

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 7.1.1 | 2 | Document timeouts | gap | SEC-176 |
| 7.1.2 | 2 | Document concurrent sessions | gap | SEC-179 |
| 7.1.3 | 2 | Document federation | gap | SEC-149 |
| 7.2.1 | 1 | Checked on the server | ok | SEC-174 |
| 7.2.2 | 1 | No static secrets | gap | SEC-156 |
| 7.2.3 | 1 | Random tokens | ok | SEC-170 |
| 7.2.4 | 1 | New token on login | part | SEC-172, SEC-173 |
| 7.3.1, 7.3.2 | 2 | Timeouts enforced | ok | SEC-176 |
| 7.4.1, 7.4.2 | 1 | Ending sessions | ok | SEC-174, SEC-175 |
| 7.4.3 | 2 | End sessions after a factor change | gap | SEC-126 |
| 7.4.4 | 2 | Logout on every page | ok | |
| 7.4.5 | 2 | Admins end sessions | gap | SEC-178 |
| 7.5.1 | 2 | Re-authenticate for sensitive changes | gap | SEC-135, SEC-605 |
| 7.5.2 | 2 | See and end sessions | part | SEC-177 |
| 7.5.3 | 3 | Extra factor for highly sensitive actions | plan | SEC-605 |
| 7.6.1 | 2 | Provider and app lifetimes | gap | SEC-148 |
| 7.6.2 | 2 | Explicit action for a session | ok | |

### V8 Authorization

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 8.1.1 | 1 | Document the rules | ok | SEC-210 |
| 8.1.2 | 2 | Rules per field | part | SEC-210 |
| 8.1.3, 8.1.4 | 3 | Context | n/a | |
| 8.2.1, 8.2.2 | 1 | Functions and objects | ok | SEC-202 |
| 8.2.3 | 2 | Fields | ok | SEC-207, SEC-208 |
| 8.2.4 | 3 | Adaptive controls | n/a | |
| 8.3.1 | 1 | On the server | ok | SEC-201 |
| 8.3.2 | 3 | Changes apply at once | ok | SEC-205, and see SEC-542 |
| 8.3.3 | 3 | The caller's rights | ok | SEC-446 |
| 8.4.1 | 2 | Tenants | n/a | One organisation per installation |
| 8.4.2 | 3 | Layers around admin | gap | SEC-608 |

### V9 Self-contained tokens

We issue none. ID tokens are checked by `go-oidc`, and were not walked line by line (SEC-142).

### V10 OAuth and OIDC, client part

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 10.2.1 | 2 | PKCE or state | ok | SEC-140 |
| 10.2.2 | 2 | Mix-up | n/a | SEC-147 |
| 10.2.3 | 3 | Few scopes | ok | Only what login needs |
| 10.5.1 to 10.5.4 | 2 | Nonce, stable subject, issuer, audience | ok | SEC-140 to SEC-143 |
| 10.5.5 | 2 | Forced logout through back-channel logout | n/a | Applies once SEC-148 is built |

The rest of V10 is for authorization servers, which we are not until phase 7.

### V11 Cryptography

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 11.1.1 | 2 | Key policy | gap | SEC-705 |
| 11.1.2 | 2 | Inventory | ok | Chapter 8 |
| 11.1.3 | 3 | Discovery tools | n/a | |
| 11.1.4 | 3 | Post-quantum plan | later | SEC-730 |
| 11.2.1 | 2 | Known libraries | ok | |
| 11.2.2 | 2 | Agility | part | SEC-702, SEC-713 |
| 11.2.3 to 11.2.5 | 2, 3 | Strength, constant time, fail safe | ok | SEC-701 |
| 11.3.1 to 11.3.4 | 1 to 3 | Modes, AEAD, nonces | ok | SEC-703 |
| 11.3.5 | 3 | Encrypt then MAC | n/a | GCM |
| 11.4.1 to 11.4.3 | 1, 2 | Hashes, passwords | ok | SEC-100, SEC-704 |
| 11.4.4 | 2 | Keys from passwords | n/a | |
| 11.5.1, 11.5.2 | 2, 3 | Random values | ok | SEC-700 |
| 11.6.1, 11.6.2 | 2, 3 | Signatures, key exchange | ok | |
| 11.7.1 | 3 | Memory encryption | n/a | |
| 11.7.2 | 3 | Little data in memory | part | |

### V12 Secure communication

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 12.1.1 | 1 | TLS versions | part | SEC-626, SEC-728 |
| 12.1.2 | 2 | TLS suites | part | SEC-728 |
| 12.1.3 | 2 | Client certificates | n/a | |
| 12.1.4, 12.1.5 | 3 | Revocation, ECH | op | |
| 12.2.1 | 1 | TLS for clients | part | SEC-257, SEC-415 |
| 12.2.2 | 1 | Public certificates | op | |
| 12.3.1 | 2 | TLS in and out | part | SEC-625 |
| 12.3.2 | 2 | Certificates checked | part | SEC-416, SEC-625 |
| 12.3.3 | 2 | TLS between parts | plan | SEC-624 |
| 12.3.4 | 2 | Trusted internal certificates | part | SEC-625 |
| 12.3.5 | 3 | Mutual authentication | plan | SEC-729 |

### V13 Configuration

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 13.1.1 | 2 | Document connections | part | SEC-507 |
| 13.1.2, 13.1.3 | 3 | Limits of connections | later | Level 3 |
| 13.1.4 | 3 | Rotation schedule | gap | SEC-723 |
| 13.2.1 | 2 | Service authentication | plan | SEC-503 |
| 13.2.2 | 2 | Least privilege | ok | SEC-617 |
| 13.2.3 | 2 | No default credentials | ok | SEC-620 |
| 13.2.4, 13.2.5 | 2 | Allow-lists out | plan | SEC-431, SEC-433 |
| 13.2.6 | 3 | Follow connection config | later | Level 3 |
| 13.3.1 | 2 | Secrets management | part | Docker or Kubernetes secrets, SEC-619, SEC-720. A vault comes with SEC-712 |
| 13.3.2 | 2 | Least privilege for secrets | ok | SEC-619 |
| 13.3.3 | 3 | Isolated crypto | plan | SEC-712 |
| 13.3.4 | 3 | Rotation | gap | SEC-713, SEC-723 |
| 13.4.1 to 13.4.7 | 1 to 3 | Hardening | ok | SEC-621, SEC-622 |

### V14 Data protection

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 14.1.1 | 2 | Classes | ok | Chapter 1 |
| 14.1.2 | 2 | Rules per class | part | SEC-008 |
| 14.2.1 | 1 | Nothing sensitive in URLs | part | SEC-422 |
| 14.2.2 | 2 | No server caches | ok | SEC-253 |
| 14.2.3 | 2 | No third parties | ok | SEC-900 |
| 14.2.4 | 2 | Apply the rules | part | SEC-008 |
| 14.2.5 | 3 | Web cache deception | ok | SEC-253 |
| 14.2.6 | 3 | Least data in answers | part | SEC-211 |
| 14.2.7 | 3 | Retention | gap | SEC-831 |
| 14.2.8 | 3 | File metadata | gap | SEC-334 |
| 14.3.1 | 1 | Clear the browser at logout | gap | SEC-265 |
| 14.3.2 | 2 | No caching | ok | SEC-253 |
| 14.3.3 | 2 | Nothing sensitive in browser storage | gap | SEC-265, SEC-266 |

### V15 Secure coding and architecture

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 15.1.1 | 1 | Time limits for fixes | gap | SEC-1007 |
| 15.1.2 | 2 | SBOM | gap | SEC-1006 |
| 15.1.3 | 2 | Heavy functions | part | SEC-274 |
| 15.1.4, 15.1.5 | 3 | Risky parts listed | gap | SEC-1008 |
| 15.2.1 | 1 | Components up to date | part | SEC-1002, SEC-1003 |
| 15.2.2 | 2 | Defences for heavy functions | gap | SEC-112, SEC-231 |
| 15.2.3 | 2 | No test code | ok | |
| 15.2.4 | 3 | Expected repositories | ok | SEC-1001 |
| 15.2.5 | 3 | Sandboxes for risky parts | plan | SEC-501 |
| 15.3.1 | 1 | Only needed fields | ok | SEC-208 |
| 15.3.2 | 2 | No redirects on outward calls | plan | SEC-431 |
| 15.3.3 | 2 | Mass assignment | ok | SEC-207 |
| 15.3.4 | 2 | Client address | ok | SEC-012 |
| 15.3.5 to 15.3.7 | 2 | Types, prototypes, parameters | ok | |
| 15.4.1 to 15.4.3 | 3 | Concurrency | ok | Race detector, locks |
| 15.4.4 | 3 | Fair use of resources | part | SEC-112 |

### V16 Logging and errors

| Req | L | Short | State | Line |
|---|---|---|---|---|
| 16.1.1 | 2 | Inventory | ok | Chapter 9 |
| 16.2.1 to 16.2.4 | 2 | Metadata, time, places, format | ok | SEC-803, SEC-840 |
| 16.2.5 | 2 | Rules for sensitive data | part | SEC-820, SEC-821, SEC-422 |
| 16.3.1, 16.3.2 | 2 | Logins, refusals | ok | SEC-801, SEC-802 |
| 16.3.3 | 2 | Security events | part | SEC-800, SEC-656 |
| 16.3.4 | 2 | Errors | ok | SEC-844 |
| 16.4.1 | 2 | Log injection | ok | SEC-840 |
| 16.4.2 | 2 | Protected logs | part | SEC-811, SEC-814 |
| 16.4.3 | 2 | A separate system | later | SEC-832 |
| 16.5.1 to 16.5.4 | 2, 3 | Safe errors | ok | SEC-224, SEC-506, SEC-628, SEC-842 |

### V17 WebRTC

Not used.

### Open at Level 1 or 2

Level 2 is the aim, so these are what stands between us and it, leaving out what is planned or partly met.

- **Gaps:** 2.4.1, 3.5.5, 6.1.1, 6.1.2, 6.2.11, 6.5.2, 7.1.1, 7.1.2, 7.1.3, 7.2.2, 7.4.3, 7.4.5, 7.5.1, 7.6.1, 11.1.1, 14.3.1, 14.3.3, 15.1.1, 15.1.2, 15.2.2.
- **Chosen to wait, and so a known exception:** 5.4.3 (antivirus) and 16.4.3 (a separate log system).

## 12.2 OWASP Top 10:2025

| Category | What stops it | What is still open |
|---|---|---|
| A01 Broken Access Control | SEC-200 to SEC-209, the matrix, the route walker | SEC-542 |
| A02 Security Misconfiguration | SEC-615 to SEC-623, safe defaults | SEC-257, SEC-501 |
| A03 Software Supply Chain Failures | SEC-1000 to SEC-1002 | SEC-1003 to SEC-1006, SEC-1015 |
| A04 Cryptographic Failures | Chapter 8 | SEC-124, SEC-636, SEC-713 |
| A05 Injection | SEC-220, SEC-300, SEC-250 | SEC-546 |
| A06 Insecure Design | Chapter 1, specs with a study | SEC-006 |
| A07 Authentication Failures | Chapter 2 | SEC-134, SEC-135, SEC-156 |
| A08 Software or Data Integrity Failures | SEC-810, and SEC-352 in wave G | SEC-814, SEC-1015 |
| A09 Security Logging and Alerting Failures | SEC-800 to SEC-803 | SEC-655, SEC-656, SEC-832 |
| A10 Mishandling of Exceptional Conditions | SEC-628, SEC-842 | SEC-506 |

## 12.3 NIS2, as the Dutch Cyberbeveiligingswet

Article 21(2) names ten measures. The duty falls on the organisation, never on the software, so a product can only help. Whether an organisation falls under the law is a legal question. *Lawyer.*

| Measure | What Diffluence offers | What stays with the customer |
|---|---|---|
| (a) Risk analysis and security policies | This blueprint as input | The analysis and the policies |
| (b) Incident handling | The audit log, and ending sessions. The runbook is SEC-657 | The process and the reporting |
| (c) Business continuity, backup, disaster recovery | Backup and restore scripts, chapter 7.3. Targets of 4 hours come in phase 3 (SEC-639) | Running backups, testing them, the crisis plan |
| (d) Supply chain security | Chapter 11. An SBOM is still missing (SEC-1006) | Judging us as a supplier |
| (e) Acquisition, development, maintenance | Chapter 11, and `SECURITY.md` | Applying updates in time |
| (f) Assessing effectiveness | The checks in chapter 11. The penetration test comes before 1.0 | Its own tests and audits |
| (g) Cyber hygiene and training | Pushing people to stronger login, chapter 2.6 | Training |
| (h) Cryptography | Chapter 8 | Disk encryption and keys |
| (i) HR security, access control, asset management | Chapter 3, and accounts through SCIM | HR processes and the asset list |
| (j) Multi-factor authentication, secured communications | Chapter 2 | Requiring it, and TLS at the proxy |

## 12.4 ISO/IEC 27002:2022 and BIO2

BIO2 builds on ISO/IEC 27001 and 27002:2022, which order 93 controls in four themes. The ISO text is paid for and was not read for this study, so this table stays at the level of themes. The earlier blueprint promised a mapping table with 1.0, and a table per control is that deliverable.

| Theme | Controls | Where to look |
|---|---|---|
| Organizational | 37 | Chapters 1, 10, 11. Most of this is the operator's |
| People | 8 | Mostly the operator's. Chapter 2.6 helps |
| Physical | 14 | The operator's |
| Technological | 34 | Chapters 2 to 9 |

## 12.5 GDPR Article 32

| Measure | In Diffluence |
|---|---|
| (a) Pseudonymisation and encryption | Chapter 8. Analytics without names in wave H. Page text is not encrypted (SEC-726) |
| (b) Confidentiality, integrity, availability, resilience | Chapters 3, 7, 9 |
| (c) Restoring in time | Chapter 7.3 |
| (d) Testing and evaluating | The checks in chapter 11, the restore drill, the penetration test |

## 12.6 Not mapped yet

- The essential requirements in the annexes of the Cyber Resilience Act: not read for this study (SEC-1040).
- ISO/IEC 27002 per control (see 12.4).
