# 06.2: Accounts, login and sessions

Each surface below has three parts: what can go wrong, what stops it today, and what is still needed. The numbered lines are the last two together.

## 2.1 Passwords

**What can go wrong:** guessing and credential stuffing, cracking a stolen hash, a flood of logins that exhausts memory, learning which addresses have an account, locking someone out on purpose.

- **SEC-100** · built · Passwords are hashed with Argon2id, 19 MiB, 2 passes, 1 lane, a random 16-byte salt and a 32-byte key. The parameters are in each hash, and a weaker hash is upgraded at the next login (ASVS 11.4.2).
- **SEC-101** · built · A tampered hash cannot exhaust the server: stored parameters outside sane limits are refused.
- **SEC-102** · wave F · The minimum length is 15 characters, by the product owner's decision (ADR-026). NIST SP 800-63B-4 asks for 15 when a password is the only factor. The code still says 12. Existing passwords stay valid (ASVS 6.2.1).
- **SEC-103** · built · A password can be 1024 bytes long, far over the 64 characters ASVS 6.2.9 asks for. The limit stops a very long password from becoming a denial of service.
- **SEC-104** · built · No rules about character types, and no forced changes on a schedule. NIST forbids both (ASVS 6.2.5, 6.2.10).
- **SEC-105** · wave F · New passwords are checked offline against a list of leaked passwords, up to 50 MB, from SecLists (MIT, ADR-026). NIST requires a blocklist (ASVS 6.2.4, 6.2.12).
- **SEC-106** · new · The check also refuses words from the context: the instance name, the person's own name and address, and `diffluence` (ASVS 6.1.2, 6.2.11). Belongs in wave F, with SEC-105.
- **SEC-107** · new · Passwords are normalised to Unicode NFC before hashing, as NIST advises, so the same password typed on two systems is the same password. Old hashes need a fallback at login. Low. Belongs in wave F.
- **SEC-108** · built · Login answers with one message, and an unknown address costs the same time as a known one, through a dummy hash. A disabled account gets the same answer (ASVS 6.3.8).
- **SEC-109** · partly · Failed logins are limited to 10 per account and 50 per address in 15 minutes. The counters live in memory per node, so a restart clears them and two nodes count apart. Shared counters belong in phase 3 (ASVS 6.3.1).
- **SEC-110** · new · Medium. NIST asks to stop an authenticator after at most 100 failures in a row, and that count has to survive a restart. Store it in the database when SEC-109 is done.
- **SEC-111** · new · Anyone who knows an address can keep that person out of password login for 15 minutes, because the limit counts per account. Passkeys and single sign-on still work. ASVS 6.1.1 asks to document how brute force is stopped *without enabling malicious lockout*. Decide and document. Low.
- **SEC-112** · new · Medium. Nothing limits how many password hashes run at once. Each takes 19 MiB, and 50 tries per address means many addresses can use a lot of memory. A small queue in front of hashing keeps memory bounded (ASVS 15.2.2). Belongs in phase 3.
- **SEC-113** · new · Medium. `POST /me/totp/disable` checks the password but has no limit, unlike the routes for changing a password and starting TOTP. Someone with a stolen session can use it to guess the password. Add the same limit. Belongs in wave F.
- **SEC-114** · built · Password fields use `type="password"` and the right `autocomplete`, and paste is not blocked (ASVS 6.2.6, 6.2.7).
- **SEC-115** · built · Changing a password needs the current one, and ends all other sessions (ASVS 6.2.3).
- **SEC-116** · partly · An administrator sets a new password for someone, so the administrator knows it, and it can stay in use. ASVS 6.4.6 (Level 3) wants a reset without that. Once wave F has reset mail, this route should send a link when mail is set up, and the person must choose their own password (ASVS 6.4.1).

## 2.2 Authenticator codes and recovery codes

**What can go wrong:** a code used twice, a stolen database that reveals seeds, guessing codes, recovery codes that are cracked, a phished code.

- **SEC-120** · built · TOTP seeds are sealed with AES-256-GCM and bound to the person, so a sealed value cannot be moved to another row.
- **SEC-121** · built · A code works once: the time step must be newer than the last one used. One step of clock difference is allowed either way, and only the server's clock counts (ASVS 6.5.1, 6.5.5, 6.5.8).
- **SEC-122** · built · The second step needs a one-time challenge from a correct password. It dies after 5 wrong codes or 5 minutes.
- **SEC-123** · built · Setting up an authenticator needs the password, so a stolen session alone cannot do it. Nothing takes effect until a first code is confirmed.
- **SEC-124** · partly · Recovery codes are random and single-use, and using one is audited. But each has 80 bits and is stored as a plain SHA-256. ASVS 6.5.2 says secrets under 112 bits must be salted and hashed with a password hashing function. Medium. Make them 128 bits, which keeps a plain hash acceptable. Belongs in wave F.
- **SEC-125** · built · TOTP stays offered next to passkeys. It is not phishing resistant, so passkeys are the route people are pushed towards (SEC-160).
- **SEC-126** · new · Medium. Removing a factor leaves other sessions running, both when a person does it and when an administrator resets it. ASVS 7.4.3 asks to at least offer ending them. An attacker with a session should not outlive the reset of the account. Belongs in wave F.
- **SEC-127** · wave C · A person is told by mail when a password, factor or passkey changes (ASVS 6.3.7). Spec 010 builds the mail, and spec 013 the events. Check that the message exists for each change.

## 2.3 Passkeys

**What can go wrong:** a stolen security key without a PIN opens the account, a stolen session adds its own passkey, a passkey for another site is accepted, a list of credentials shows who has an account.

- **SEC-130** · built · The relying party id and origin come from the public address. A passkey made for another site does not work here.
- **SEC-131** · built · A login uses discoverable credentials, so no address is typed and nothing shows whether an account exists.
- **SEC-132** · built · Challenges are random, single-use, stored as a hash, and last 5 minutes (ASVS 6.7.2).
- **SEC-133** · built · No attestation is asked for, so nothing identifies the make of a device. Disabled accounts are refused for every method in `OpenSession`.
- **SEC-134** · new · **High.** User verification is not required. The code says a passkey is "possession plus a local check", and skips the second factor because of it, but `BeginDiscoverableLogin` sets no requirement, and the library only enforces a check when the session says *required*. A stolen security key without a PIN is therefore enough on its own. Pass `webauthn.WithUserVerification(protocol.VerificationRequired)` at login, and require it at registration. Wave F counts a passkey as a second factor, so this belongs in wave F, with a test and a mutation line.
- **SEC-135** · new · **High.** Adding or removing a passkey needs no proof of who you are, while TOTP does. A stolen session can add a passkey, and then stays in after the session ends. Spec 013's list of `Sensitive` routes does not name these routes. Add `POST /me/passkeys/begin`, `DELETE /me/passkeys/{id}` and turning off a factor to that list (ASVS 7.5.1).
- **SEC-136** · new · A falling signature counter is ignored. Synced passkeys often report 0, so refusing would break them, but a counter that goes down on a key that counts is a sign of a clone. Read how the library reports it, and audit it. Low.
- **SEC-137** · later · NIST does not allow synced passkeys at its highest level, AAL3. A setting that accepts only device-bound keys, using the backup flags, can come when a customer needs it.
- **SEC-138** · wave F · Removing the last way in is refused, or clearly warned about. A person without a password who removes their only passkey has no way back but an administrator.

## 2.4 Login with OpenID Connect

**What can go wrong:** a forged callback, an attacker's login slipped into a victim's browser, an account taken over through an e-mail address, a session that outlives the one at the provider, a provider that becomes administrator.

- **SEC-140** · built · The authorization code flow with PKCE (S256), a one-time `state`, and a `nonce`. RFC 9700 asks for these (ASVS 10.2.1, 10.5.1).
- **SEC-141** · built · A cookie ties the flow to the browser that started it, so an attacker's login cannot be finished in someone else's browser.
- **SEC-142** · built · The library checks signature, issuer, audience and expiry of the ID token (ASVS 6.8.2, 10.5.3, 10.5.4).
- **SEC-143** · built · Accounts link on issuer and subject, never on an e-mail address alone. A local account with a password is never linked (ASVS 6.8.1, 10.5.2).
- **SEC-144** · built · After login the redirect is fixed, so there is no open redirect (RFC 9700, ASVS 3.7.2).
- **SEC-145** · built · A groups claim can only add people to groups that came from that same source, and never to a local group with the same name.
- **SEC-146** · built · The client secret comes from the environment or a file, and is not stored in the database.
- **SEC-147** · later · Only one provider can be set up, so mix-up attacks do not apply. RFC 9700 requires a defence as soon as there are more: store the issuer per request, and check it (ASVS 10.2.2).
- **SEC-148** · new · Medium. A session here keeps running when the provider ends it, up to the idle limit, unless SCIM disables the person. OpenID Connect Back-Channel Logout closes this gap (ASVS 7.6.1, 10.5.5). A new public route with strict token checks: signature, `iss`, `aud`, `events`, and no `nonce`. Belongs after wave F.
- **SEC-149** · new · Write down how session lifetimes at the provider and here relate (ASVS 7.1.3). Belongs in `docs/operations/identity-providers.md`.
- **SEC-150** · wave F · A step-up for accounts of a provider goes through a fresh login with `prompt=login`. The response must then be checked for `auth_time`, or a provider that ignores the prompt passes (ASVS 6.8.4). Spec 013 does not say so yet.
- **SEC-151** · new · A group from the provider can hold *administrator*, so whoever controls the provider controls this instance. That can be intended. The console should show it plainly, and the audit log shows who joined. Low. Belongs in wave F.
- **SEC-152** · new · `OIDC_AUTO_CREATE` is on by default, so everyone at the provider can get an account. The install guide should say so. Refuse an `http` issuer outside local testing. Low.
- **SEC-153** · new · The cookie for the flow has no `__Secure-` prefix on https (ASVS 3.3.1). Low. Belongs in wave F.

## 2.5 Provisioning with SCIM

**What can go wrong:** a stolen token creates or disables people, moves them into groups, or reads the list of everyone.

- **SEC-155** · built · Tokens have 256 bits, are stored hashed, are shown once, can be revoked, and each call is audited with the token's name.
- **SEC-156** · new · Medium. Tokens never expire. RFC 7644 says bearer tokens must have a limited lifetime that the provider can check. Add `expires_at`, a date chosen at creation (default one year), a warning in the console, and a rotation that lets two tokens overlap (ASVS 7.2.2, 13.3.4). Belongs in wave F, with the other tokens.
- **SEC-157** · built · Filters that are not understood are refused, not ignored, and SCIM cannot set a password.
- **SEC-158** · built · A disabled person loses their sessions at once.
- **SEC-159** · partly · A SCIM call runs as a system caller, so the check that keeps one administrator does not apply: a provider can disable every administrator. The way back is `user create-admin` on the host. Accepted, and it should be in the guide.
- **SEC-161** · partly · RFC 7644 advises against personal data in query strings, and providers filter on e-mail with `GET`. We keep to what providers send. Query strings are never logged, so the address stays out of our logs. Proxies must do the same, and the guide should say so.

## 2.6 Pushing people to stronger login

The product owner's decision (ADR-026): push people towards a second factor and passkeys, and administrators towards an identity provider and passkeys.

- **SEC-160** · wave F · A person without a second factor or passkey gets a clear prompt after login and in their account. They can put it off, but not switch it off for good.
- **SEC-162** · wave F · An administrator sees a warning in the console while no identity provider is set up, and while they have no passkey themselves.
- **SEC-163** · wave F · An administrator can require a second factor for administrators or for all members (ACC-015). NIST requires at least one phishing-resistant option at its middle level, which passkeys are.
- **SEC-164** · new · Administrators get the requirement by default on a new instance: a grace period, then the console needs a second factor. This is the logical end of ADR-026. Decision for the product owner.

## 2.7 Sessions

**What can go wrong:** a stolen cookie, a session that lives too long, a session that survives a change that should end it, a token in a log.

- **SEC-170** · built · Session tokens have 256 bits from the system's random source, and only a SHA-256 of them is stored, so a database leak does not leak sessions (ASVS 7.2.3).
- **SEC-171** · built · The cookie is `HttpOnly`, `SameSite=Strict`, and on https `Secure` with the `__Host-` prefix (ASVS 3.3.1 to 3.3.4).
- **SEC-172** · built · A new token at every login, so a session cannot be fixed in advance (ASVS 7.2.4).
- **SEC-173** · wave F · A step-up must issue a new token as well (ASVS 7.2.4 counts re-authentication). Spec 013 stores `reauth_at` and does not say so. Add it.
- **SEC-174** · built · Sessions are checked on the server on every request, and ending one works at once (ASVS 7.2.1, 7.4.1).
- **SEC-175** · built · Logout, a password change, a reset and disabling an account all end sessions (ASVS 7.4.2).
- **SEC-176** · partly · The idle limit is 8 hours and the absolute limit 7 days, both configurable. NIST advises 1 hour and 24 hours at its middle level, so these defaults are those of its lowest level. ASVS 7.1.1 asks to document a deviation with a reason. Decision for the product owner.
- **SEC-177** · partly · A person sees their sessions and ends any of them (ASVS 7.5.2). One button that ends all others is missing. Belongs in wave F.
- **SEC-178** · new · An administrator can end the sessions of one person or of everyone, without disabling them, for example during an incident (ASVS 7.4.5). Belongs in wave F.
- **SEC-179** · new · There is no limit on sessions per person. That is fine, but ASVS 7.1.2 asks to write it down, and the Sessions screen shows them. Low.
- **SEC-180** · wave F · Background requests do not keep a session alive, or the polling of wave C would make the idle limit meaningless (spec 013, D8).
- **SEC-181** · later · Sessions are not tied to an address or browser. OWASP says such a binding cannot be relied on alone, and phones change address all day. A change of country can become a warning later.
- **SEC-182** · built · Session tokens are never in a URL, and the access log leaves query strings out.

## 2.8 The life of an account

**What can go wrong:** a default account, a reset link that can be guessed or used twice, a link built from a forged `Host` header, a mail bomb, a reset that skips the second factor, an invited guest who stays forever.

- **SEC-185** · built · There are no default accounts. The first administrator is made from the command line, with the password from standard input (ASVS 6.3.2).
- **SEC-186** · built · The last administrator cannot be removed by a person, and *system administrator* can only be granted by one.
- **SEC-187** · wave F · A reset link has a hashed random token, works once, lasts an hour, and always gets the same answer with equal timing. It ends all sessions and logs nobody in (ASVS 6.4.3, 6.3.8).
- **SEC-188** · wave F · The second factor still applies after a reset. Losing a factor goes through an administrator, who must check who is asking (ASVS 6.4.3, 6.4.4). The guide must say how.
- **SEC-189** · wave F · Links in mail are built from `DIFFLUENCE_PUBLIC_URL`, never from the `Host` header, or a forged request could send a victim a reset link to an attacker's site. Spec 013 should say this in so many words, with a test.
- **SEC-190** · wave F · Reset and invitation are limited per address and per account, so nobody can be flooded with mail.
- **SEC-191** · wave F · Invitations expire. Self sign-up is off by default and limited to chosen domains. Guests can have an end date.
- **SEC-192** · new · An address can only change through the provider today: a local account has no way to change it, not even through an administrator. If wave F adds that, it needs a step-up, a confirmation at the new address and a notice to the old one, because the address is what `Login` matches on.
