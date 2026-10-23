# Accept the validators' additive status members (pinsSha256, attempt, next_attempt_at) before they are written

Branch `pv2/rb12-status-pins` on `origin/main` ab842f5. One source file changed in `pkg/server`, plus tests. No migration. `database.RequiredSchema` is unchanged.

## DEPLOY ORDER (hard precondition)

1. **This PR first** (proofs_service).
2. **Then the validators** that write the new members: `pv2/rb11-dbfix` (pkg/finality/status.go, `pinsSha256`) and `pv2/rb12-p0-live` (pkg/finality/status.go, `attempt` / `next_attempt_at`).

If the validators roll first, every `proof_finality_status` row carrying `pinsSha256` is answered `finality_status_malformed{reason=not_a_status_document}` for its level, and the whole `GET /api/v1/proofs/{id}/finality` of that proof is a 500 `FINALITY_SUMMARY_UNUSABLE`. Rolling this PR first is safe against old rows: a summary of rows without the new members is byte for byte what it was.

## Defect

`levelFromStatus` (pkg/server/finality_handlers.go) decodes the stored `certen-finality-status/1` document with `DisallowUnknownFields` into a fixed struct. The validators' builder status gains the top-level member `pinsSha256` (64 lowercase hex, the sha256 of the pins the builder judged under; written last, omitted when the writer holds no pins) and the named state of a failed ask gains the fields `attempt` and `next_attempt_at`. Finding 1 of RB12-DBFIX.

## Claim

proofs_service accepts, validates and serves the validators' status document including those members; it stays strict about everything else; anything it cannot accept is a named state, never read as absent.

## Change (pkg/server/finality_handlers.go)

- `statusDocument` gains `pinsSha256`, kept raw so that a member that is present but `null`, `""`, a number, upper case, `0x`-prefixed or of another length is refused with its **own** reason `bad_pins_sha256` (same state `finality_status_malformed`, naming the level) instead of being read as absent. Absent stays valid (old rows, writers without pins). `DisallowUnknownFields` is kept: any other member, and `attempt` / `next_attempt_at` placed at the document top level, are still `not_a_status_document`.
- `attempt` / `next_attempt_at` are fields of the named state (`state.fields`), as the validator writes them (Canonical form: numbers as decimal strings). The state was already carried through as an opaque object, so they were already tolerated; they are now **validated where they appear**: `attempt` must be a canonical decimal string of an integer 1..2^64-1 (reason `bad_attempt`), `next_attempt_at` an RFC 3339 string (reason `bad_next_attempt_at`). A state without them, or whose fields are not an object, is read as it always was.
- Served summary (`certen-finality-summary/1`): each level object gains an optional last member `pinsSha256`, so a consumer that verifies under the pins IT trusts can see whether the builder's pins differ from its own. On a level read from a status row it is that row's identity. On a level that has an addendum (present whatever its row says, unchanged) it is shown only when the row is that addendum's own verified status (class `verified`, `seq` equal to the addendum's seq) with a well-formed identity; a stale, foreign or malformed row shows no identity and neither changes the level nor fails the summary.

### Decision on the summary wire format (evidence)

Passing the identity into the summary is additive and safe for both consumers:

- api-gateway (`origin/main`, src/services/finality-view.ts `finalityFromSummary` / `levelView`) reads named members of the summary and its levels (`str('state')`, `str('judgedAt')`, ...) and ignores any other member; there is no strict schema on the summary (`git grep "strict()"` over finality-view.ts and proofs.client.ts: none). The client caps the body at 262,144 bytes; the member adds about 80 bytes per level. The gateway therefore drops the member until it chooses to surface it (not in this PR).
- The TypeScript verifier (certen-sdk `packages/verify`) does not read the summary or the status document at all: `git grep "finality-summary\|finality-status\|pinsSha256"` over `origin/main` and the `origin/pv2/*` branches of certen-sdk finds nothing in `packages`. It verifies the proof file under the pins the consumer supplies.
- No other repo under certen/ reads `certen-finality-summary` or the status table except the validators (writer), api-gateway and this service.

## Tests

`pkg/server/finality_status_pins_test.go` (8 tests) and `pkg/server/finality_status_pins_pg_test.go` (build tag `pgtest`, 1 test).

Fail-before at ab842f5 in a throwaway detached worktree (output in the RB12-PROOFS-SVC check-in): the pins and back-off documents are answered `not_a_status_document` / 500 `FINALITY_SUMMARY_UNUSABLE`, 15 malformed-pins shapes at two levels get the shape reason instead of their own, back-off fields are not validated, a stored level shows no identity; the pgtest fails the same way over the validators' real DDL. Pass-after on this branch.

- documents shaped as the validator writes them (pending, unavailable with the back-off fields, verified; with and without pins) through `SummarizeFinality` (exact summary bytes) and through the HTTP handler;
- old rows (no new members) summarise as before, no `pinsSha256` anywhere;
- malformed pins: empty, null, number, true, array, object, upper case, mixed case, 63 / 65 characters, non-hex, `0x` prefix, padded, trailing newline, non-ASCII digit, at both levels, and a verified-class row;
- strictness not loosened: an extra member beside valid pins, `pins`, `pinsHash`, `pinsSha256Hex`, and `attempt` / `next_attempt_at` as document members are all `not_a_status_document`;
- back-off fields: 16 malformed shapes by reason, well-formed extremes accepted;
- stored level: pins only from its own verified status (seq match, class verified, no unknown member, well-formed identity);
- pgtest: rows inserted into the real `proof_finality_status` jsonb column (re-rendered by PostgreSQL: keys re-ordered, spaces after separators), read by the real repository, served by the real handler; stored levels with the vector addenda.
- Hand mutation of the new conditions (16 mutants over finality_handlers.go): 15 killed (one needed an added case); the 16th (`|| fields == nil`) was equivalent (a nil map reads as empty) and the redundant condition was removed.

`go vet ./...`, `go vet -tags pgtest ./...`, `go test ./... -count=1` and `go test -tags pgtest ./... -count=1` (shared test PostgreSQL, own database, dropped) are green.

## Production shape

The live store holds rows of the OLD shape only (the new members are not written yet): those are covered by the existing exact-bytes tests and the explicit no-member test. The new shapes are taken from the validator code (status.go `Document()`, state.go `Canonical()` with numbers as decimal strings) and the validator's own test (`status_pins_test.go`). The live host was down when this was written, so no live sample was taken.

## Findings (not fixed here)

- The document decode matches member names case-insensitively (Go `encoding/json`), as it always did for every member; `PINSSHA256` is read as `pinsSha256` (its value is validated and the member is served under its canonical name). Tightening this would change the behaviour for all members.
- The validator's `noteAttempt` adds `attempt` / `next_attempt_at` to ANY unavailable or refused state, while the validator catalog (`testdata/finality/states.json`) lists them as optional only for `finality_build_timeout`; this service validates them for every state code.
- api-gateway drops the new member (`levelView` copies only named members); surfacing `pins_sha256` there is a separate change.
