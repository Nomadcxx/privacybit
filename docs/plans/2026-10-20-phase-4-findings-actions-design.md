# Phase 4: findings and actions

**Status:** approved in dialogue, 2026-10-20
**Depends on:** Phase 3 audit grader, D1 §9, D6 §8

## Scope

Build findings from the current pure audit result and attach only actions whose
data and behavior are defined. Keep the evaluator read-only; this phase does
not execute commands, write settings, persist dismissals, or require root.

## Finding rules

- A matched installed product is a finding when its evaluated grade is amber or
  red. Red maps to high severity and amber to medium severity.
- A missing expectation is a finding only when the grader judged it. Its
  authored severity, grade, and explanation are retained.
- Unmatched inventory and expectations that cannot be judged remain visible in
  inventory and coverage; they do not become findings.
- A catalogue-known Flatpak installed at both system and user scope gets a
  separate low-severity informational finding. It does not change the product
  or category grade.
- Findings for the same product and installation scope merge observations so
  package, binary, and desktop corroboration does not create duplicate cards.
  Flatpak system and user installs remain separate.

## Action rules

- Every finding has a tier 0 `inform` action that opens its explanation.
- A tier 3 `launch` action is added for each catalogue alternative with a
  command or HTTPS upstream route for the selected distribution family. It
  carries the rendered instruction; privacybit never runs it.
- Tier 1 is withheld until dismissal persistence exists. Tier 2 is withheld
  until concrete, reversible settings are represented in the catalogue.
  Neither appears as an inert placeholder. Tier 4 remains Phase 8.
- Distribution rendering uses only the closed D6 family and package-manager
  mapping. Package names and upstream URLs are validated before they are placed
  in a copyable action.

## Ownership

`internal/audit` owns finding derivation because findings are facts about the
graded result. `internal/render` owns catalogue-backed install instructions so
the CLI and findings share one renderer. A later audit runner supplies the
current distribution family and composes findings into the snapshot.
