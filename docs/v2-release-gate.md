# v2.0.0 release gate

Target: v2.0.0, private deployment first. This is a gate, not a readiness claim.

## Required product closure

- [ ] Workspace recurring execution exposes authenticated, sanitized running/blocked/error status, including discovery failures; bounded capacity, fairness, backoff, cancellation and no late history are exercised.
- [ ] Company/Tenant and department/Workspace management is usable via authenticated HTTP and UI. Every selected scope is authorized against live membership; legacy default APIs remain default-only.
- [ ] Checkmate, Gatus, Uptime Kuma and Nagios preview and apply target an authorized Workspace, preserve CAS and credential boundaries, and supported imported loopback checks actually execute. Reuse each adapter's existing supported formats; incompatible semantics must explicitly block activation, not be silently downgraded.
- [ ] A fresh install automatically has the stable default Tenant and default Workspace (initial display names `default` / `default`); setup does not require company/department input. Upgrade and restart do not duplicate defaults. Users may rename display names without changing ownership IDs or history. Single-Workspace users enter monitoring directly; authorized scope switching appears only when useful and never confers permission.
- [ ] Workspace-owned incidents/events and durable notification intents deliver PROBLEM and RECOVERY independently across scopes, survive restart, and do not leak into default stores. Unsupported policy semantics are rejected explicitly. Delivery crash windows must be documented rather than called exactly-once.

## Frozen-tree product scenarios

These rounds cannot start counting until the product closure above passes and the integrated product tree is frozen. A product change resets the rounds.

1. [ ] Fresh install: default company/department, real setup/login, monitoring, channels, reports, imports, responsive navigation and restart, with independent data/ports/browser state.
2. [ ] Adversarial ownership: multiple companies/departments, equal target addresses with distinct credentials, distinct roles, cross-scope ID rejection, revoked membership, scoped import and independent PROBLEM/RECOVERY delivery.
3. [ ] Previous-version upgrade: isolated copy/generated old-version database, stable monitor/history identity, restart, invalid config/CAS, source loss and recovery where supported. Never touch production data.

Record source fingerprint, raw outputs and scenario-specific assertions. Three repetitions of one mocked fixture do not satisfy this gate.

## Package and publication

- [ ] Build immutable v2.0.0 candidate archives; test extracted binaries and included assets on each claimed runnable platform. Cross-compilation is not execution.
- [ ] Repeat relevant real-browser/API, restart and upgrade scenarios against extracted artifacts; distinguish in-process tests from external-binary tests.
- [ ] Capture screenshots from the exact accepted binary and verify runtime version, binary hash, localized README references and preserved installation commands.
- [ ] Commit/push integrated changes; verify exact remote SHA and successful CI.
- [ ] Publish only once all gates pass; verify downloaded release asset hashes against accepted archives.

## Prior evidence, not reusable final acceptance

Commit 811336c passed remote CI run 41363337606. The v1.5.7-rc.1 Windows/amd64 archive passed scoped extracted-package acceptance and supplied six candidate screenshots. Those results establish the pre-v2 baseline only; they do not validate subsequent product edits or complete the independent-instance gate.

All tests use disposable data, loopback probe/notification receivers or local log channels. Real customer endpoints, notification recipients and existing production executables remain outside scope.
