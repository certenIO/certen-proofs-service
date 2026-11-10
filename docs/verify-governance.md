# POST /api/v1/proofs/verify/governance

This endpoint verifies governance evidence. It does not report a stored flag and it does not accept evidence because its
members are present. Every `valid: true` is the verdict of the validator's own offline verifier, run in this process over the
evidence bytes.

## The verifier is the validator's code (DAX-F2)

The offline verifier lives in `certenIO/certen-validator` (public), whose `go.mod` declares the module path
`github.com/certen/independant-validator`. This service imports it; nothing is copied or re-written.

| Level | Evidence | Code that decides |
|---|---|---|
| G0, G2 | a governance receipt (stored `level_json.receipt`, or `proof_data.receipt`) | `proof.GovReceiptEvidence.VerifyMerkle` (it walks the path with the one `ReceiptVerifier` of the liteclient) |
| G1 | the portable proof v2 document (`certen-proof-v2-accumulate-portable/1`, evidence 2.0 or 2.1) | `proofv2.VerifyPortable`, then the pinned-incarnation check below |
| stored G0/G2 | `governance_proof_levels.level_json` of the proof | `proof.GovernanceLevelsFromStorage`, the reader `cmd/proofverify --governance` uses |

How the module is resolved (decision, options considered):

1. **Go module at a pinned commit (chosen).** `go.mod` has `replace github.com/certen/independant-validator =>
   github.com/certenIO/certen-validator v0.0.0-20261026210941-15814ad5579e` and the same for the validator's nested
   `accumulate-lite-client-2/liteclient` module (a dependent does not inherit the validator's local replace). The repository
   is public, so `docker build .` fetches it from the Go module proxy with no credentials; `go.sum` pins every byte. No
   `replace` points at a local path.
2. Byte-identical copy of the validator's files into this repo: rejected. `pkg/proof` is one Go package of about 70 files
   that import the liteclient, the Accumulate SDK and gnark; a subset cannot compile alone and a copy of the whole package
   is a second place to drift. A copy needs a drift test that a pinned module already gives for free.
3. Re-implement the checks here: rejected (the "lookalike" the finding forbids).
4. Call the offline CLI (`proofverify`) as a subprocess: rejected. It reads the validator's PostgreSQL schema and ships as a
   separate binary; the service would have to carry and trust a second executable.

Drift guard. `docs/verify-governance-provenance.json` records the source repository, the commit and the sha256 of every
validator source file the endpoint depends on (the module cache holds those exact bytes). `TestGovernanceVerifierIsTheRecordedValidatorCode`
fails when `go.mod` moves to another commit or any listed file differs from the record, and the workflow
`.github/workflows/governance-verifier.yml` runs it on every pull request and push. Moving the pin is a deliberate act: change
`go.mod`, read the validator's diff of those files, regenerate the record, run the tests.
Vectors: `TestGovernanceValidatorVectorsGiveTheValidatorsVerdicts` runs the validator's own certify1 conformance vectors
(`pkg/server/testdata/governance/vectors`, byte for byte, sha256 checked against the validator's manifest) and requires the
validator's verdict for each.

Operational cost of the choice: the binary now links the validator's dependency graph (tens of MB larger than the 10 MB it was),
a cold `go mod download` takes minutes, and `go.mod` states Go 1.26.10 (the first toolchain with the standard-library fixes
that `.github/workflows/govulncheck.yml` enforces).

## Request

`POST` with a JSON object; unknown members are refused. Body limit 32 MiB.

| Member | Meaning |
|---|---|
| `governance_level` | `G0`, `G1` or `G2` (required) |
| `proof_id` | a stored proof (UUID). Exactly one of `proof_id` and `proof_data` |
| `proof_data` | evidence in the request. G0, G2: `{"receipt": {level?, start, anchor, localBlock, entries}}`. G1: `{"portable": <portable proof v2 document>}`. No other member is accepted |
| `pinned_incarnation` | hex32 Accumulate incarnation the caller trusts, from the caller's own record (G1). Optional when the service has `CERTEN_PINNED_INCARNATION` set |

`CERTEN_PINNED_INCARNATION` (service environment, hex32): the incarnation the operator trusts. If it is set and unusable the
service refuses to start; a request-level pin wins over it.

## Response

HTTP 200 carries a verdict. HTTP 4xx/5xx is a refusal of the request or a storage failure. No response other than a 200 with
`state: "verified"` carries `valid: true`.

```json
{ "valid": false, "level": "G1", "state": "incarnation_mismatch", "reason": "...", "details": { }, "verified_at": "2026-11-10T12:00:00Z" }
```

| `state` | `valid` | Meaning |
|---|---|---|
| `verified` | true | the verifier returned no error and (G1) the proven incarnation equals the pinned one, the validator set verdict is `verified` and no fact is unproven |
| `evidence_absent` | false | nothing to verify: no `receipt`/`portable` member, `null`, no stored level or document |
| `summary_only` | false | a stored level records a conclusion without a receipt path; nothing was checked and nothing is claimed |
| `evidence_malformed` | false | the evidence does not parse, does not compose (for example a stored spine shorter than the document needs) or has unknown members |
| `evidence_invalid` | false | the verifier refused it; `reason` carries the verifier's text (for example `certify_link_missing{0}: ...`) |
| `evidence_unproven` | false | the verifier accepted it and reports facts it could not establish; `details.unproven` names them |
| `validator_set_unverified` | false | the validator set was asserted, not derived and bound to the incarnation |
| `incarnation_not_pinned` | false | G1 evidence verifies against the incarnation it names, but no incarnation was pinned, so which chain it is about is not established; `details.incarnation` is the value to pin |
| `incarnation_mismatch` | false | the evidence is about another incarnation than the pinned one |
| `level_not_stored` | false | the proof has no stored row for the requested level |

HTTP errors: `400` `INVALID_REQUEST`, `INVALID_LEVEL`, `INVALID_EVIDENCE_SOURCE` (neither or both of the sources),
`INVALID_PROOF_ID`, `INVALID_PINNED_INCARNATION`; `405`; `413` `REQUEST_TOO_LARGE`; `500` `INTERNAL_ERROR` (storage
failure, or an unusable configured pin). A storage error text never reaches the client.

## What `valid: true` establishes, and what it does not

- G1 (`proofv2.VerifyPortable`): the document's evidence, replayed offline from the genesis of the pinned incarnation through
  the Directory's major blocks, proves the transaction receipt, the certified root, the authority pages and the validator
  quorum. This is the strong claim. It needs a pin; without one the answer is `incarnation_not_pinned`.
- G0, G2 (`VerifyMerkle`): the receipt's leaf recomputes to the receipt's own stated anchor. It does NOT show that the anchor
  is a real Accumulate anchor; that is what the G1 document proves. The response says so in `details.does_not`. This is
  PARTIAL by design, not a default.
- A stored `verified` flag is never read, in either direction: real evidence verifies with the flag false, and tampered or
  summary-only evidence is refused with the flag true (`TestGovernanceStoredVerifiedFlagIsNeverTheAnswer`).
