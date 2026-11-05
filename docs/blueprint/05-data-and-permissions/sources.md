# 05: Sources

What was read for this study on 2026-11-05, and for what. Everything is in our own words. No source code of another wiki was read, and no other project of the product owner's was used as a source or example.

## Our own

- `docs/blueprint/03-functional-requirements.md` and all of `03-functional/`, above all chapters 1 to 3, 7 to 9.
- Specs 002 and 008 to 016.
- `internal/permissions`, `internal/db/migrations`, `internal/pages`, `internal/spaces`, `internal/attachments`, `internal/trash`, `internal/search`, `internal/exchange`, `internal/identity`, `internal/audit`, `docs/api/routes.txt`.
- The branches `wave-c`, `wave-d`, `wave-f` and `space-home` as they stood that day, read only.

## PostgreSQL 18 documentation

- [Row Security Policies](https://www.postgresql.org/docs/current/ddl-rowsecurity.html): default deny, owner bypass, covert channels through unique and foreign key checks, races in policies that read other tables.
- [Transaction Isolation](https://www.postgresql.org/docs/current/transaction-iso.html): what a statement sees under Read Committed.
- [Constraints](https://www.postgresql.org/docs/current/ddl-constraints.html): what foreign keys do on delete.
- [Text search limitations](https://www.postgresql.org/docs/current/textsearch-limitations.html): the limit of 1 MB per `tsvector`.

## Access control

- [NIST SP 800-192, Verification and Test Methods for Access Control Policies/Models](https://csrc.nist.gov/pubs/sp/800/192/final), the abstract: a model must be checked against what its authors meant.
- [NIST SP 800-162, Guide to Attribute Based Access Control](https://csrc.nist.gov/pubs/sp/800/162/upd2/final), the abstract: decisions from attributes of subject, object and action. Our rule is of that shape, with relationships for groups and the tree.
- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html): deny by default, check on every request, protect lookup ids, cover static resources, test authorization.

## Annotations and privacy

- [W3C Web Annotation Data Model](https://www.w3.org/TR/annotation-model/): Text Quote Selector, and its warning about copying text.
- [GDPR Article 5](https://gdpr-info.eu/art-5-gdpr/): storage limitation, data minimisation.
- [GDPR Article 17](https://gdpr-info.eu/art-17-gdpr/): erasure, and its exceptions. Not legal advice.
