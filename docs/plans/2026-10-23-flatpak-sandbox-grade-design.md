# Flatpak Sandbox Grade Design

Issue: [#13](https://github.com/Nomadcxx/privacybit/issues/13)

## Decision

Grade effective Flatpak sandbox permissions on an axis separate from the app's
catalogue grade. Keep each installed copy distinct by installation scope. The
owner approved these generic bands:

| Grade | Evidence |
|---|---|
| Red | Host filesystem access, all devices, or shared networking. |
| Amber | Bounded broad access such as home files, host configuration, X11, or USB. |
| Green | Complete, classified permission evidence without a red or amber grant. |
| Unknown | Missing output, a failed query, invalid output, or a permission the grader does not classify. |

The overall grade includes the worst sandbox evidence. The report keeps the
catalogue grade in its own field. Sandbox permissions can be graded for an
unmatched app because they are independent evidence; the app remains unmatched
and unrated.

## Evidence source and scope

Use Flatpak's own `info --show-permissions` command for each row returned by
`flatpak list`. Invoke it with the matching user, system, or named installation
and the listed app id, branch, and architecture. Flatpak resolves metadata,
global overrides, and per-app overrides when reporting the deployed app
context, so privacybit uses Flatpak's result instead of reimplementing override
precedence or reading override files directly.

The command uses argv arguments and the current user's privileges. If a
permission query fails, keep the installed app and mark that copy's sandbox
grade unknown with a reason. Each app copy needs one query, matching Flatpak's
per-app permission interface.

## Grading and coverage

The detector carries the permission output as an unavailable-aware signal. The
audit layer parses only recognized permission groups and keys. It records
recognized grants and short reasons, never the complete metadata output: the
output can include environment values that do not belong in a report.

An unclassified permission or incomplete parse yields unknown, even when a
known red grant is also present; the recognized grant remains visible as
evidence. A fully classified red grant yields red. A fully classified amber
grant yields amber. A fully classified set without a risk grant yields green.
The aggregate sandbox grade uses the worst copy grade. Report counts and a
partial-coverage flag show which copies the audit could not classify.

Privacybit adds a `sandbox` object to schema 1. It contains
the aggregate grade, applicability, bounded per-copy assessments, coverage
counts, and recognized grants. Human CLI output and the sysc-plugins panel show
the same information. The panel gives sandbox posture its own section after app
categories, keeping the catalogue grade and permission grade visibly distinct.
When the panel caps visible copies, it places highest-risk grades first and
uses app id and scope as stable tie-breakers.

## Compatibility and limits

The new field leaves snapshot schema 1 intact. The plugin accepts snapshots
with or without it, so an older privacybit binary continues to render. The
sysc-shell Control Centre uses the overall grade and opens the plugin panel for
details, so it needs no change.

The audit inspects only the current user's installations. It reports Flatpak
network visibility as unknown and does not infer it from sandbox permissions.
