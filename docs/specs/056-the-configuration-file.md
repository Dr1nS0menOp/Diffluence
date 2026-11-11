# Spec 056: the configuration file and `config check`

Wave O4, the fourth wave of spec 048. It builds the configuration lines of the study of blueprint 07 (`docs/blueprint/07-operations/06-configuration.md`, OPS-099 to OPS-102), and closes OPS-006. The product owner approved a YAML reader on 2026-11-10 (ADR-040); spec 048 decision D4 chose `go.yaml.in/yaml/v3` in strict mode.

## Study

The study (Yin et al., SOSP 2011; Xu et al., FSE 2015 and OSDI 2016) gives three lessons: most failures from configuration are a single wrong value with a poor message, most settings are never changed, and settings only read in an emergency must be checked at start. This wave checked the code against them:

| Point | Found in the code | Consequence |
|---|---|---|
| Settings | `internal/config/config.go` reads every setting from `DIFFLUENCE_*`, each also from `*_FILE` | Twelve-factor already; a file is only an addition |
| Errors | `Load` collects every problem and names the variable | The message is good; a file needs a line number as well |
| Both a value and its `_FILE` | Refused | Keep, and apply the same rule inside the file |
| A command that checks without starting | None; `doctor` loads the configuration but needs a database | An operator cannot test a change before a restart |
| Emergency settings | The mail server is first reached when a mail goes out, the identity provider on the first login, the publish folder on the first publish | The mistake shows when it hurts most (Xu et al., OSDI 2016) |
| YAML 1.1 traps | Not relevant yet | `no`, `off` and `012` must never be read as `false` or octal |

`go.yaml.in/yaml/v3` 3.0.5 was already in `go.sum` through another module; this wave makes it direct. It is MIT and Apache-2.0 (the files `LICENSE` and `NOTICE` of the module), and needs no other module to build.

## Decisions

1. **One file, named by `DIFFLUENCE_CONFIG_FILE`.** No file is read unless that variable is set, so installations from before change nothing.
2. **The keys are the names of the environment variables, in lower case, without `DIFFLUENCE_`.** A mapping may nest them: `smtp: {host: x}` is `smtp_host`. Writing the same setting twice, also once flat and once nested, is refused, as is an unknown key. Each refusal names the key and its line.
3. **The order is default, then the file, then the environment.** A container platform can still override any value. `config check` says which source won, and says when the environment overrides the file.
4. **Values are read as the text written, never as YAML's guess of a type.** `no` stays the text `no`, which a true/false setting refuses; `012` stays `012`, never an octal number. A list or mapping where one value belongs is refused.
5. **Secrets.** A secret may come from a path (`smtp_password_file: /run/secrets/smtp`), like `*_FILE` in the environment. A secret written in the file itself is allowed only when the file can be read by its owner alone; otherwise the start is refused, and the message says to run `chmod 600` or use a `_file` key. The secrets are the database addresses, `smtp_password`, `oidc_client_secret`, `collab_token`, `metrics_token` and `pdf_password`.
6. **One validator.** `config.Load` stays the only place that checks settings. The server, `doctor` and `config check` all call it, so they cannot disagree. Every setting `Load` asks for is a known key, so a setting added later is known in the file without a second list.
7. **`diffluence config check`** loads the configuration as a start would, prints every setting with the value in effect and its source, with each secret shown as `(set)` and the password in a database address replaced, then tries the settings that are otherwise only used later (decision 8). It exits 1 when the configuration is refused, and 0 otherwise; a probe that fails is a warning, because the outside system may be down for its own reasons. `--offline` skips the probes.
8. **Settings used later are tried at start (OPS-101)**, without side effects, each within 5 seconds: the mail server's name resolves and its port accepts a connection, the identity provider's discovery document loads, and the publish folder takes a file that is removed at once. At `serve` a failure is a warning in the log; `config check` and `doctor` print it.
9. **The list of settings (OPS-102).** No setting is removed or renamed in this wave: installations set them. The guide gets a short list of what every installation sets, and marks the rest as rarely needed.

## Plan

Each step: failing test first, then the change, then a mutation.

1. Reading the file: unknown keys, duplicate keys (flat and nested), values that are not one value, the line in every message, the YAML 1.1 traps.
2. The order of sources and `_file` keys in the file; a key and its `_file` both in the file.
3. The mode of a file with a secret in it.
4. The report of sources and masked values.
5. The probes.
6. The command `config check`, and `doctor` printing the probes.
7. The example file `deploy/diffluence.example.yaml`, `install.md`, `compose.yaml`, the changelog, the OPS lines, `docs/dependencies.md`.

## Tests

- `TestAnUnknownKeyInTheFileIsRefusedWithItsLine`, `TestASettingWrittenTwiceIsRefused`, `TestAListWhereOneValueBelongsIsRefused`, `TestYAMLGuessesAreNotTaken` (`no`, `off`, `012`).
- `TestTheEnvironmentWinsOverTheFile`, `TestTheFileWinsOverTheDefault`, `TestASecretFromAPathInTheFile`, `TestAKeyAndItsFileVariantBothInTheFileAreRefused`.
- `TestASecretInAFileOthersCanReadIsRefused`, and the same file with mode 600 is accepted.
- `TestTheReportNamesEverySourceAndHidesSecrets`: no secret text appears in the output.
- `TestTheProbesWarnAndNeverFail` with a closed port, an unreachable provider and a folder that cannot be written.
- `TestConfigCheckExitsOneOnABadFile` in `cmd/diffluence`.

## Load budget (ARC-238)

One file read at start and at `config check`. The probes run once at start, in the background, each bounded by 5 seconds; nothing runs per request.

## What one machine cannot prove

A mail server and an identity provider that answer: the probes are tested against a closed port and a refused address. The real servers are tried on the test VM in wave 7.

## Open for the product owner

None.
