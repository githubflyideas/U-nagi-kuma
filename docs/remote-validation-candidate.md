# Remote validation candidate — not a release

This branch is an unfinished integration snapshot uploaded for remote CI because the local development disk is nearly full. It is not v1.5.7 release acceptance, and must not be merged/tagged/published merely because CI passes.

Implemented checkpoints include UI/report/security improvements, bounded Checkmate/Gatus import adapters, Zone execution/synchronization foundations, and company/Workspace repositories, encrypted credentials/configuration and bounded execution. Workspace automatic lifecycle has only a targeted test so far.

Known incomplete areas include explicit Workspace runtime error/status reporting, blocked-config/credential/backoff and capacity/fairness coverage, scoped incidents/notifications, full department HTTP/UI ownership, and tenant-aware importer activation. Some older checkpoint documents intentionally describe intermediate states; inspect later checkpoint documents and actual code before claiming support.

Existing Actions cover Go tests/race, vet, lint, browsers, operating-system tests, binary smoke, distribution builds and container smoke. Those jobs provide remote regression evidence only. The user's final gate remains three distinct complete product-instance scenarios on the final tree followed by complete applicable acceptance against actual extracted release packages. No release workflow is triggered by this candidate branch.
