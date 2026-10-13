# Documentation maintenance guide

## Required synchronization

| Code change | Check and update |
|---|---|
| User feature, permission, page, or error behavior | Help Center and both detailed-doc languages |
| Component, flow, or security boundary | Both `.drawio` sources, SVG exports, README architecture |
| Helm value, port, protocol, or direction | Both READMEs, network guides, Help Center, Chart README |
| Install, upgrade, uninstall, or migration | Installation guides and README quick start |
| API/CRD contract | API reference entry, examples, and usage guides |
| Built-in rule behavior | Rule guidance and Polaris reference boundary |

Chinese is the primary content source, but English meaning, links, parameters, and diagrams must match before merge. Commit editable `.drawio` sources with the SVGs used by README. Review against actual charts and code. Any deferred synchronization must be disclosed with its risk.

## Completion checks

Check internal links, locale key parity, both Help Center languages, Helm lint, frontend tests, and frontend build. Documentation must never include tokens, passwords, real environment addresses, or credentials.
