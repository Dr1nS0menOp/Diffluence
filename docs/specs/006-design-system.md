# 006: Design system and font import

Status: built
Phase: 2

## Problem

The first interface worked, but looked unfinished. The product owner wants the intuition of a mature wiki, in a look of our own, and wants that look to hold for every future change without depending on anyone remembering it. An organisation may also want its own font.

## Behaviour

**The look.** Every screen follows `docs/design/README.md`: the Helder style and layout, the Rail editor, and a page tree with collapsible branches.

**The page tree.** Branches show guide lines. A reader can collapse a branch; it stays collapsed in that space across pages and reloads, until the reader opens it again or goes to a page inside it. The sidebar can be hidden as a whole.

**Asking first.** Discarding a draft, restoring a version, and deleting a page, a space or a trashed item ask for confirmation in a dialog of our own. Adding a link uses one too. The browser's own dialogs never open.

**The font.** Diffluence uses the system font of the device. Under Administration, Settings, Appearance an administrator can import one font in WOFF2 or WOFF format of at most 2 MiB. It applies to everyone at once, also on the login page. "Use the system font" removes it. A file that is not such a font is refused with a clear message. A font that the browser cannot use changes nothing: the system font stays.

## Permissions and audit

Importing and removing a font need *administrator*, and produce the audit event and outbox event `settings.font_changed`, in one transaction with the change.

Reading which font is in use, and the font file itself, are public: the login page needs them before anyone has logged in. They reveal only the file name of the font and the font file, which holds no data of the installation. The stored location of the file is not exposed.

The file is recognised by its first bytes, never by its name. It is served as `font/woff2` or `font/woff` with `nosniff`, from our own origin, so the Content Security Policy stays `font-src 'self'`. The browser sanitises fonts before use.

## Acceptance criteria

- [x] Colours, type, spacing and corners come only from the tokens, and a violation fails `make check`
- [x] Text has WCAG AA contrast in light and dark, and a violation fails `make check`
- [x] No browser dialogs, no emoji or glyph icons, no inline styles, and each is a failing check
- [x] Every screen has an approved picture, and a screen without one fails `make check`
- [x] A screen that drifts from its picture fails the browser tests
- [x] Each rule of the check is proven to fail on a violation
- [x] A collapsed branch stays collapsed across pages and a reload, and can be opened again
- [x] Going to a page inside a collapsed branch reveals it
- [x] The sidebar can be hidden and brought back
- [x] A link is added without a browser dialog
- [x] A member cannot import a font; a file that is not a font, or is over the limit, is refused
- [x] A real font applies after import, also on the login page before logging in, without a CSP violation
- [x] Removing the font returns to the system font, and the old file is queued for deletion
- [x] Font changes are audited and published
- [x] A font is only cached for long under its versioned address

## How the product owner tests it

Open a space, collapse a branch of the tree, click through a few pages and press F5: it stays collapsed. Edit a page and look at the strip and the tools. Delete a page: a dialog asks first. Switch your computer to dark mode. Under Administration, Settings, import a `.woff2` font: the whole app changes, also the login page in a private window. Then press "Use the system font".

## Provenance

Own design, made with the product owner in the design rounds of 2026-11-04. WOFF file signature from the public W3C WOFF and WOFF2 specifications. No product was consulted.
