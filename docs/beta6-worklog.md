# Beta.6 Development And Verification

## Objective

Review the complete repository, research useful additions, implement the selected
features, fix findings and repeat verification, then commit, push and publish
`v1.0.0-beta.6`. Passing tests are evidence, not a guarantee of zero defects.

## Scope

- [x] Repository-wide review: configuration, providers, probes, reports,
  persistence, authentication, notifications, scheduling, updates, HTTP endpoints,
  frontend, installer, CI, packaging and documentation.
- [x] Research native Anthropic Messages and Gemini generateContent, compatible
  with the existing explicit protocol selection and request budgets.
- [x] Implement native text and streaming probes, paginated discovery, usage,
  diagnostics and frontend configuration without changing legacy Type behavior.
- [x] Implement server-side filtering and pagination for diagnostics and incidents.
- [x] Implement administrator audit history with bounded retention, filtering,
  actor attribution and no request bodies, credentials or raw error payloads.
- [x] Add regression tests for features and all confirmed review findings so far.
- [x] Run full local checks, browser workflows and desktop/mobile visual review.
- [ ] Run Linux race, Windows, dependency and installer checks in GitHub Actions.
- [ ] Publish bilingual notes (complete Chinese, then complete English), tag and
  packages; verify downloaded artifacts, migration, main/tag and clean worktree.

## Release Constraints

- Preserve existing configuration and published tags/assets.
- New protocols are opt-in; Provider Type remains a branding field.
- Do not add file-based environment loading or automatic paid model requests.
- Keep Beta prerelease semantics; do not update Docker `latest`.
- Keep preview services and user data separate from test databases.
- Record deployment acceptance limits honestly.

## Baseline

- Reviewed starting commit: `d297a9b` (`v1.0.0-beta.5`).
- Working tree was clean when this work started.
- Existing beta.5 has two OpenAI-compatible protocols, monitoring snapshots,
  backups, rules, incidents and scheduling. Native protocol selection, searchable
  monitoring history and actor-attributed audit history are missing.
- Provider Type is not a protocol selector. Automatically reinterpreting existing
  `anthropic` or `gemini` branding would break compatible proxy configurations.

## Research And Findings

Primary research:

- Anthropic's official Go SDK at
  `c9ebe447ac92c91748af817c265398e5d81ca49f`:
  `message.go`, `model.go`, `client.go`. Messages, terminal reasons, SSE event
  ordering, cumulative usage, cache input counts and `after_id` pagination.
- Google's official Gen AI Go SDK at
  `3a7595eccddf2530209ac9eb044802c71c246eb0`:
  `types.go`, `models.go`, `api_client.go`. Generation endpoints, API-key header,
  SSE, `supportedGenerationMethods`, page tokens and thinking usage.
- OWASP Logging Cheat Sheet:
  attribution, authentication and administrative actions, bounded retention,
  exclusion of passwords/tokens/session values and control-character handling.
- Sources and user-facing limitations: `native-protocols.md`, `audit.md`.
  Direct Claude documentation returned a region-unavailable page; it was not
  treated as API evidence. Vendor SDK sources were retrieved through GitHub.

Confirmed findings and fixes:

1. Chat JSON responses copied negative upstream token counts into stored usage.
   A regression failed before the fix; Chat now uses the validated shared usage
   conversion and overflow-checked summation.
2. Full checks synthesized discovery recovery for disabled/paused providers.
   A regression failed before the fix; only enabled, participating providers
   contribute synthetic discovery observations.
3. The new cancellation test's fake POST handler did not drain the request body,
   preventing server cleanup. A timed targeted run identified the blocked
   goroutines; the fixture now drains the body and has explicit cleanup.
4. Embedding probes overwrote shared usage validation, incorrectly marking
   contradictory totals, negative totals/output or unexpected output as known.
   Four regression cases failed before the fix. Embeddings now declare only
   their inherent zero output while retaining shared consistency validation.
5. Linux browser gates exposed Radix's stale Escape listener closing the outer
   Provider dialog while a select layer registers. The new deterministic browser
   regression also failed locally before the fix. A fail-closed postinstall
   patch with version/hash checks rechecks the layer stack in both module formats;
   see upstream issue #4143 and `frontend.md` for removal criteria.
6. The first Docker run after that fix exposed a packaging omission: the
   Dockerfile ran `npm ci` before copying the postinstall patch runner. The
   Docker build now copies that single script before dependency installation;
   the full frontend source is still copied afterwards.
7. Installer gates stalled in unconditional package installation before running
   tests, despite ShellCheck being included in the hosted runner image. The
   workflow now checks the existing installation first and bounds fallback
   downloads, the setup step and the job. Syntax, lint and all installer tests
   remain required.

Review inventory:

- Backend: configuration/revisions, lifecycle and task cancellation, selection,
  protocol/auth/usage/stream boundaries, safe outbound transport, reporting,
  notifications/rules/debounce, scheduling, budgets, persistence/retention,
  backups, metrics, users/sessions, HTTP permissions, exports and updater.
- Frontend: route authorization/session refresh, shared API/lifecycle helpers,
  dashboard, model selection/forms, task/report ordering, settings, users,
  notifications, monitoring/audit, exports, credentials, update handoff and
  shared desktop/mobile components.
- Delivery: installer language/address handling, archive/path/ownership checks,
  snapshots and recovery, CI/release/Docker packaging, README and all current
  guides. Historical Release notes retain their original version's facts.
- Corrected stale README protocol support, missing route documentation and the
  distinction between global alerts and provider-scoped independent rules.

Implementation notes:

- Protocol selection remains explicit, preserving legacy Provider Type branding.
- Every additional native discovery page reserves budget; no partial catalogs.
- Unknown final streaming usage and special cache pricing keep token counts but
  do not produce misleading known costs.
- Diagnostic/incident cursors use descending IDs, parameterized exact filters
  and exclusive end timestamps. The old monitoring snapshot remains compatible.
- Audit is an explicit operation allowlist with verified actor snapshots and
  status classification. It has no raw request or response fields, and uses a
  separate bounded write context. It is best-effort, not transactional or
  tamper-proof; asynchronous acceptance is not task completion.

## Verification Evidence

Fresh local evidence during implementation:

- `go test ./... -count=1 -timeout=180s`: PASS after the final embedding fix.
- `go vet ./...`: PASS after the final embedding fix.
- `npm test --prefix frontend`: 39 PASS, including patch integrity/idempotence
  and rejection of unreviewed source/version changes.
- `npm run build --prefix frontend`: PASS after native/history/audit UI integration.
- `npm run test:browser --prefix frontend`: PASS for the complete combined suite,
  including native protocol, audit and all prior regressions.
- `npm run test:monitoring --prefix frontend`: PASS with cursor/filter/late error
  regression cases at desktop, mobile and 320px.
- Audit Playwright cases: PASS at 320px and 1440px, including filters, cursors,
  stale requests, refresh failure, accepted results and ordinary-user isolation.
- Screenshots include desktop/mobile native, diagnostics and audit views;
  audit captures disable theme transitions.
- Installer isolation suite: 39 PASS with Git Bash on Windows.
- `npm audit --prefix frontend --audit-level=low`: zero vulnerabilities.
- Clean `npm ci` applies the version/hash-checked Radix patch. The deterministic
  Escape registration regression failed before it and passed afterwards at
  320px/1440px, including three Chromium repetitions and the full browser suite.
  A candidate patching dependency introduced audit findings and was removed;
  the final patch runner uses only Node built-ins and adds no dependencies.
- The first Docker packaging gate failed because the postinstall runner was not
  present during `npm ci`; this is fixed and remains a required remote gate.
- `govulncheck` v1.8.0 on the final tree: no vulnerabilities found. The Go proxy
  timed out during tool resolution; built the same pinned, cached module and
  ran the scanner normally against the vulnerability database.
- `git diff --check`: PASS; generated HTML line endings normalized.
- `scripts/tests/test_release_upgrade.cjs`: PASS with the published beta.5
  Windows binary and the candidate. Preserved account/session/CSRF, settings,
  private status, history, usage, tasks, incidents and old backups; exercised
  native JSON/SSE, paginated history, audit, new backups and restart persistence.
  Rebuilt the binary and repeated acceptance after the final embedding fix.

Publication Gates

This file records pre-publication evidence. The remaining checkboxes above must
be verified against the corresponding GitHub Actions runs and published tag,
not inferred from local tests:

- The initial main CI #59 and release #40 passed Go/installer/build gates but
  failed in the native browser regression while waiting for model discovery.
  The request waiter could reject before the click was awaited, hiding the
  useful interaction error. The regression now joins both promises, waits for
  select focus/closure and saves failure screenshots in CI artifacts. CI #60
  and release #41 exposed the prematurely closed dialog, identifying the
  dependency defect above. Local Chromium repetitions alone do not establish
  that the Linux gate is fixed.
- Push main and wait for Linux race, Windows, dependency and installer gates.
- Create beta.6 only after those gates pass, then verify the Release, all eight
  assets/checksums, downloaded binary/version/migration and Docker prerelease tag.
- Preserve older tags/assets and do not publish Docker `latest`; no paid
  upstream calls. Real systemd/macOS and long-duration acceptance remain outside
  the available local environment and are documented in the Release notes.
