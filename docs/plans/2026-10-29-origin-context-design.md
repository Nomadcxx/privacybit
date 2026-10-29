# Origin Context Design

Issue: [#22](https://github.com/Nomadcxx/privacybit/issues/22)

## Decision

Show package and remote origin as provenance context. Origin never changes an
application, category, or overall grade. This follows the owner's choice on
issue #22 and resolves the conflict between D1/CONTEXT and D6.

The existing detector already records an origin source name and whether that
source is marked official. The report will preserve those two values for each
grouped application. It will deduplicate by the pair, so observations that name
the same source with different official status remain visible. Flatpak copies
remain grouped by installation scope, as they are today.

## Report and display

Schema 1 gains an optional `origins` list on each app, plus shown, total, and
truncated counts. The report sorts origins by source and then official status,
shows up to eight per app, and marks any truncation. It sanitizes each source
label before serialization: non-printing characters become spaces, whitespace
is collapsed, and labels stop at 80 UTF-8 bytes without splitting a rune.

The human CLI, TUI app detail, and sysc-plugins panel show the origin and label
whether the source is official or not marked official. Each UI calls this
"origin context" so users do not read it as grade evidence. The plugin applies
the same bounds and sanitization when decoding input. Missing origin fields
remain valid, keeping older privacybit snapshots readable.

The audit grader will stop treating a contradicted origin signal as a grade
input. Other contradiction signals keep their existing monotone behavior.

## Scope

This change does not add origin detection, classify new repositories, infer
trust from an origin name, or change sysc-shell. It exposes facts the existing
detectors already collect and leaves origin coverage unavailable where a
source cannot report it.
