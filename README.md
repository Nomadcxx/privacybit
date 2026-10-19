# privacybit

A privacy audit for Linux desktops.

privacybit inspects what is installed, what is running, and what is actually
going over the network, then reports a graded breakdown by category with a
stated reason for every grade and a concrete action for every finding.

```console
$ privacybit audit
```

## What it checks

- **Applications, by category.** Browser, VPN, password manager, chat, email,
  office, PDF, media — each installed application graded `green`, `amber`,
  `red`, or `unknown`, with the reason shown.
- **Live network activity.** Processes connected to known telemetry and
  analytics endpoints, identified by executable rather than by name.
- **VPN reality.** Not "is a tunnel interface up" but whether the default route
  goes through it, whether DNS leaks to the ISP, and whether IPv6 escapes. A
  connected VPN with leaking DNS reports `red`, because a green light that means
  nothing is worse than an obviously-off VPN.
- **The defaults you did not choose.** Default browser, default search engine,
  analytics daemons, update checkers, systemd timers that phone home.

## What it will not do

- **Make any network calls.** No update check, no crash reporting, no analytics,
  no version ping. This is enforced by test, not by convention.
- **Read outside your own scope.** No other users' processes, no keys, and no
  browser profile contents beyond the specific preference keys a grade depends on.
- **Guarantee anonymity.** privacy, security and anonymity are different
  properties and are not interchangeable. This grades privacy.

## Design

See [`docs/plans/2026-10-20-privacybit-design.md`](docs/plans/2026-10-20-privacybit-design.md).

Two rules shape almost everything:

- **Derived signals are monotone.** A measurement may only push a grade toward
  more risk, never improve it. Telling someone their setup is better than
  expected on the strength of a heuristic is an unearned claim.
- **Absence is a finding.** No password manager installed is `red`, not empty.

## Licence

TBD.