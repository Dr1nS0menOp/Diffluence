# 07.6: Configuration

Blueprint 07 says configuration is "one YAML file and/or environment variables", with secrets from files. Diffluence reads environment variables, each with a `_FILE` variant. The product owner approved a YAML reader on 2026-11-10. This chapter says what research teaches about configuration, and how the file should work.

## 6.1 What the research says

- **Configuration errors are a leading cause of failures.** Yin et al. studied 546 real configuration errors in commercial and open-source systems (SOSP 2011). A large share were simple mistakes in a single value, and many systems gave a misleading message or none at all.
- **Most settings are never touched.** Xu et al., *Hey, You Have Given Me Too Many Knobs!* (FSE 2015), found that most users change only a small share of the settings, and that many settings could be removed or set by the software itself. Every setting is a chance for a mistake.
- **Check everything at start.** Xu et al. (OSDI 2016, best paper) looked at *latent* configuration errors: settings for reliability that are only read when something goes wrong, such as a backup path or a fail-over address. In the systems they studied, 14% to 93% of those settings had no check at start, so the mistake showed during the very failure the setting was meant to handle. Their tool generated checks that found more than 75% of real latent errors at start. The lesson is simple and does not need their tool: read and test every setting when the process starts, including the ones only used in an emergency.

## 6.2 How the industry does it

- **The Twelve-Factor App** keeps configuration in the environment, so the same build runs everywhere. Almost every container platform follows it.
- **A file for the rest.** Long lists, nested settings and comments read better in a file. Most server projects accept both, with a fixed order: built-in default, then the file, then the environment, so a container platform can still override any value.
- **Secrets stay out of both, where possible.** The OWASP Secrets Management Cheat Sheet and the container platforms point to secret files: Docker and Kubernetes secrets, and systemd credentials (`LoadCredential=`). An environment variable can leak through process listings, crash reports and child processes.
- **YAML has traps.** YAML 1.1 reads `no`, `off` and `NO` (the country code of Norway) as `false`, and `012` as an octal number. YAML 1.2 fixed most of this. A reader for configuration should be strict: refuse unknown keys, refuse duplicate keys, and read values with the type the setting expects.
- **The Go reader.** The widely used `gopkg.in/yaml.v3` was archived as unmaintained on 1 April 2025. The YAML organisation took the code over as `go.yaml.in/yaml/v3` (MIT and Apache-2.0), a drop-in replacement that other large Go projects moved to. It supports strict decoding of known fields.

## 6.3 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| Every setting from the environment, each with `_FILE` | Built | `internal/config/config.go`, OPS-007 |
| Both set at once is refused | Built | `TestBothVariantsIsAnError` |
| A wrong shape of an installation is named at start, passwords stay out of errors | Built | `docs/operations/limits.md` |
| A configuration file | Built in spec 056 | OPS-006, OPS-099 |
| A command that checks a configuration without starting | Built in spec 056: `diffluence config check` | OPS-100 |
| Settings that only matter in an emergency checked at start | Partly: the key, database and folders are; the identity provider is looked up on first use (`internal/sso/sso.go`), and the mail server is first reached when a mail goes out | |

## 6.4 Recommendations

- **OPS-099** · built (spec 056) · **Medium.** A configuration file, `DIFFLUENCE_CONFIG_FILE`, in YAML, read with `go.yaml.in/yaml/v3` in strict mode: unknown or duplicate keys refuse the start and name the key and line. The keys are the environment names in lower case without the prefix, nested where that helps (`smtp.host`). Order: default, file, environment. A secret may be given as a path (`smtp.password_file`); a secret written in the file itself is allowed only when the file is readable by its owner alone, otherwise the start is refused. This closes OPS-006.
- **OPS-100** · built (spec 056) · **Medium.** `diffluence config check` reads the environment and the file, checks every setting as a start would, and prints the effective configuration with each secret replaced by `(set)`. It also says where each value came from: default, file or environment. Operators and the release checklist use it before an upgrade.
- **OPS-101** · partly (spec 056: warnings in the log, config check and doctor; not yet on System health) · **Medium.** Check the settings that are only used in an emergency or later, at start, without side effects: the SMTP host resolves and its port answers, the identity provider's discovery document loads, the publish folder is writable. A failure is a warning in the log and on the System health page, not a refused start, because the outside system may be down for its own reasons.
- **OPS-102** · partly (spec 056: the guide names what every installation sets; nothing removed) · **Low.** Review the list of settings against the knobs study: each setting that nobody needs to change gets a fixed value or is worked out by the software, and the guide lists the few that every installation sets.

## Sources

- Yin, Ma, Xu, Rouf, Pasupathy, Bairavasundaram: [An Empirical Study on Configuration Errors in Commercial and Open Source Systems](https://doi.org/10.1145/2043556.2043572), SOSP 2011.
- Xu, Jin, Huang, Zhou, Lu, Jin, Pasupathy: [Early Detection of Configuration Errors to Reduce Failure Damage](https://www.usenix.org/conference/osdi16/technical-sessions/presentation/xu), OSDI 2016.
- Xu, Jin, Pasupathy, Zhou, et al.: [Hey, You Have Given Me Too Many Knobs! Understanding and Dealing with Over-Designed Configuration in System Software](https://doi.org/10.1145/2786805.2786852), ESEC/FSE 2015.
- Wiggins: [The Twelve-Factor App, III. Config](https://12factor.net/config).
- OWASP: [Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html).
- systemd: [Credentials](https://systemd.io/CREDENTIALS/).
- [YAML 1.2.2 specification](https://yaml.org/spec/1.2.2/), changes from 1.1 in section 1.3.
- [go-yaml, archived notice](https://github.com/go-yaml/yaml), [yaml/go-yaml, the maintained fork](https://github.com/yaml/go-yaml).

All sources of this study are also in [sources.md](sources.md).
