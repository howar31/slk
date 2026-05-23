# Security Policy

## Supported Versions

`slk` is pre-1.0 software. Security fixes are applied to the latest released version only.

| Version | Supported |
| ------- | --------- |
| 0.7.x   | ✓         |
| < 0.7   | ✗         |

## Reporting a Vulnerability

Please report security vulnerabilities privately via GitHub's
[**Report a vulnerability**](https://github.com/howar31/slk/security/advisories/new) button
(repository **Security → Advisories**). Do **not** open a public issue for security reports.

You can expect an initial response within a few days. Once a fix is available, a GitHub
Security Advisory will be published, crediting the reporter unless anonymity is requested.

## Token Handling

`slk` stores Slack tokens in `~/.config/slk/config.toml` with `0600` permissions and never
prints or logs token strings. If you find a code path that leaks a token to stdout, stderr,
logs, or process arguments, please treat it as a security issue and report it through the
channel above rather than filing a public issue.
