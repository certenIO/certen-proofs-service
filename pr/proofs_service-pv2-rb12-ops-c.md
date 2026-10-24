# proofs_service: exact status member names, and the /v2 document served at the spec's evidence bound (RB12-OPS-C)

Base: origin/main 21593a8 (PR #17). Branch: pv2/rb12-ops-c. Deploy this BEFORE the api-gateway change of the same name (the gateway requires the Content-Length this service now sends).

## 1. Status members are read by their exact names (ledger F11)

`encoding/json` matches member names case-insensitively, so `PINSSHA256` was read as `pinsSha256`. The only strict decode of validator-written JSON in the service is `decodeStatus` (pkg/server/finality_handlers.go); `TestEveryStrictDecodeOfTheServiceIsRegisteredWithItsCaseTest` scans pkg/ and cmd/ for `DisallowUnknownFields()` and fails if another file ever uses it. Three structures are read, and a member spelled in another case is refused on each, with reason `member_name_case` and the contract's own spelling in field `member` (the stored spelling is never echoed):

- the status document (members taken from the struct tags; `TestTheStatusDocumentMembersAreExactlyTheValidatorsContract` pins them to the validator's `statusDoc`): `TestACaseVariantOfAnyStatusMemberIsRefusedByNameAndNeverRead` (every member, four variant spellings each, through `SummarizeFinality` and through the endpoint: 500 `FINALITY_SUMMARY_UNUSABLE`), `TestAnExactMemberBesideItsCaseVariantIsRefused`;
- the named state (`code`, `fields`): `TestACaseVariantOfAStateMemberIsRefusedByName`;
- the state's fields the service validates (`attempt`, `next_attempt_at`): `TestACaseVariantOfABackOffFieldIsRefusedByName`. Other field names stay free-form.

Agreement with the validator's writer: `pkg/server/testdata/finality_status/` holds nine documents written by the validator's `BuilderStatus.Document` at pv2/rb12-train2 d6a9eef1 (throwaway test, not committed), with the validator reader's `ParseBuilderStatusDocument` reading of each in `manifest.json`. `TestTheServiceReadsTheValidatorsGoldenStatusDocumentsAsTheValidatorDoes` checks each file's sha256, that every member the writer spelled is in the contract, and that the service reads class, levels, eta, seq, pins identity and the state exactly as the validator reader does.

Not changed here (validator code, routed to OPS-A2): the validator's own reader `ParseBuilderStatusDocument` is case-insensitive too (`manifest.json` records `validatorReadsCaseVariantPinsMember: true`; measured at d6a9eef1 it also reads `CLASS`). It is on a live path: `FinalityStatusRepository.GetStatus` (pkg/database/finality_status.go) calls it, and that is called by the builder's `convergeStatus` (pkg/finality/upgrader.go:1421, which decides whether to rewrite a status row) and by `proofverify --db` (cmd/proofverify/finality.go:89). It needs the same exact-name decode in the validator, with these nine documents as vectors.

## 2. The summary shape the gateway surfaces pinsSha256 from (ledger GW1, gateway side in the gateway PR)

`TestTheSummaryOfARowWithPinsIsTheGatewaysFixtureByteForByte` pins `pkg/server/testdata/finality_summary/pins_pending.json`; the gateway's `test/fixtures/finality-summary-pins.json` is a byte-identical copy (sha256 17219b5e46ecb5b2c35b9b7cbd767a91d2f4327fa4301990c06d55bd6dd52ba8).

## 3. The /v2 evidence bound is the spec bound (ledger section D)

`MaxFinalityEvidenceBytes` was 6 MiB. It is now `MaxFinalityContainerBytes` (64 MiB, V2_SPEC_S7 8.2 "evidence container <= 64 MiB", the validator's `DefaultLimits().MaxEvidenceBytes`) times `MaxFinalityAddendaPerProof` (2, one per stored level) = 134217728. An Arbitrum finalized container at max_header_chain 32768 (about 18 MB) is legal and is served. `TestEvidenceAboveTheBoundIsANamedRefusalNeverATruncation` (existing, kept): exactly the bound is served, one byte over is the named 500 `FINALITY_SECTION_UNUSABLE finality_evidence_too_large bytes=N limit=134217728`, with no evidence read; its pin of the old 6 MiB figure is replaced by the new figures and by the gateway's cap, 2 x 134217728 + `PortableRestAllowanceBytes` (16 MiB) = 285212672.

The body is written, not built (pkg/server/portable_stream.go): the same bytes `json.Marshal` of the composed document gives, with the evidence hex-encoded in 16 KiB chunks straight from the rows into the response. `TestTheStreamedBodyIsByteForByteTheComposedDocument` compares the body with `ComposePortableWithFinality(ComposeFinalitySection(...))` for the three validator vector sets, with and without the govRoot inputs, plain and gzip. `TestServingTheLargestLegalDocumentCopiesNoEvidence` serves rows holding exactly `MaxFinalityEvidenceBytes` (268,436,413 body bytes) and measures: 101,728 bytes allocated while serving, peak heap 101,368 bytes above the held rows (limit 8 MiB each); on the base commit 6.3 MB of evidence allocated 81,891,768 bytes and grew the heap 62,985,424 bytes. The rows themselves are read whole from the database before anything is written, and that is NOT one copy: measured over the real `lib/pq` repository at the maximum (two 64 MiB rows, status 200, 268,436,413 bytes), a request raises total allocation by 256 MiB and the peak heap by 256 MiB (HeapSys 527 MiB), about twice the evidence, because the driver's message buffer and the scanned copy are both live at the peak. The test above measures what serving adds to the rows, not that. The consequence for the host: a maximum-size request needs about 2 x 128 MiB of memory and the service has no memory limit in docker-compose.yml and no cap on concurrent serves of this kind, so the host must be sized for the number of maximum-size documents it may serve at once. The live figures are far below it (largest stored evidence 2,484,741 bytes, 90 portable documents, the longest 631,664 characters; read-only counts of 2026-10-24 by the independent review, checkinsV-RB12-OPSC.md).

Also in this change, each with a test: the plain response declares its exact Content-Length (a gzip response is chunked) so a reader can refuse an oversize body by name before reading it; the write deadline of the response is 30 s plus one second per MiB of body (`TestServingTheLargestLegalDocumentCopiesNoEvidence`, `TestAnSmallDocumentIsGivenTheBaseWriteWindow`), because the server's 30 s WriteTimeout (cmd/proof-service/main.go:239) cannot deliver 268 MB to a slow reader; a write that fails after the status was sent panics with `http.ErrAbortHandler` instead of ending the response as a whole document (`TestAWriteFailureMidBodyAbortsTheResponse`), and the service's recover middleware now lets that panic reach net/http, which closes the connection (`cmd/proof-service/middleware_abort_test.go`, over a real server: the client reads exactly the bytes sent and then an unexpected EOF, no error JSON written into the body, no `[panic]` log line; an ordinary panic is still the 500); and a stored document that spells its `finality` member in any case is refused as `finality_member_stored_in_document`, because the Go consumers of the document match member names case-insensitively (`TestAStoredFinalityMemberInAnyCaseIsRefused`); a stored `majors` or `govRootV3Inputs` member spelled in another case (including `majorſ`, which folds to `majors`) is refused as `portable_member_name_case member=<exact spelling>` without echoing the stored one (`TestAStoredCaseVariantOfAComposedMemberIsRefused`; the exact spellings are composed over as before); and the bound is asserted again on the rows that were read, because the size and the rows are two statements (`TestTheBoundIsAssertedAgainOnTheRowsThatWereRead`: rows adding up to exactly the bound are served, one byte more is the named 500 even when the size read before reported less).

## Compatibility

No migration, `RequiredSchema` unchanged. Responses for every existing document are byte-identical (the vector goldens). A status row a validator wrote can only be newly refused if it spells a member in a case the writer never uses.

## Before deploying (read-only, owner/ops)

The service newly refuses a status row that spells a member in another case than the validator writer does. The writer (`BuilderStatus.Document`) never does, so each of these four counts is expected to be 0 (the first three measured 0 / 0 / 0 on the production database by the independent review, read-only, 2026-10-24; the fourth is exercised on sample rows below); they were run against sample rows in the shared test PostgreSQL (valid rows count 0, rows with a variant count 1 or 2) and can be run on the production database through a read-only session:

```sql
-- top-level members outside the contract
SELECT count(*) FROM proof_finality_status s WHERE jsonb_typeof(s.state)='object'
  AND EXISTS (SELECT 1 FROM jsonb_object_keys(s.state) k WHERE k NOT IN ('format','class','levelReached','nextLevel','eta','seq','state','pinsSha256'));
-- named-state members outside code/fields
SELECT count(*) FROM proof_finality_status s WHERE jsonb_typeof(s.state->'state')='object'
  AND EXISTS (SELECT 1 FROM jsonb_object_keys(s.state->'state') k WHERE k NOT IN ('code','fields'));
-- attempt / next_attempt_at spelled in another case
SELECT count(*) FROM proof_finality_status s WHERE jsonb_typeof(s.state#>'{state,fields}')='object'
  AND EXISTS (SELECT 1 FROM jsonb_object_keys(s.state#>'{state,fields}') k WHERE lower(k) IN ('attempt','next_attempt_at') AND k NOT IN ('attempt','next_attempt_at'));
-- portable documents: a finality member in any case, or a composed member in another case
SELECT count(*) FROM proof_v2_portable p WHERE p.document <> ''
  AND EXISTS (SELECT 1 FROM jsonb_object_keys(p.document::jsonb) k WHERE lower(k) IN ('finality','majors','govrootv3inputs') AND k NOT IN ('majors','govRootV3Inputs'));
```

These are not in the validator's `pr/rb11-prod-shape-check.sh` (validator code, out of scope here).

## Tests run

`pv2gt.ps1 -Pkg ./pkg/server` (whole package) PASS; `go test -tags pgtest ./pkg/server ./pkg/database` against the shared PostgreSQL (CERTEN_PG_SCRATCH_DB) PASS; `go vet ./pkg/server ./cmd/...` clean.
