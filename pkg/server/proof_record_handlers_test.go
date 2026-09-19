package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/certen/proofs-service/pkg/database"
)

func recordsDB(t *testing.T) (*sql.DB, *database.Repositories) {
	t.Helper()
	conn := os.Getenv("CERTEN_TEST_DB")
	if conn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CERTEN_TEST_DB is required in CI")
		}
		t.Skip("CERTEN_TEST_DB not set — the proof record endpoints need the shared schema")
	}
	db, err := sql.Open("postgres", conn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	client := database.NewClientFromDB(db)
	if err := client.VerifySharedSchema(context.Background()); err != nil {
		t.Fatalf("CERTEN_TEST_DB is not a migrated shared schema: %v", err)
	}
	return db, database.NewRepositories(client)
}

func h32(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func TestProofRecordEndpoints(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	tag := uuid.NewString()
	accumTx := "records-api-" + tag

	artifact, err := repos.ProofArtifacts.CreateProofArtifact(ctx, &database.NewProofArtifact{
		ProofType: database.ProofTypeCertenAnchor, AccumTxHash: accumTx, AccountURL: "acc://records-api.acme/tokens",
		ProofClass: database.ProofClassOnDemand, ValidatorID: "api-test", ArtifactJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_artifacts WHERE proof_id = $1`, artifact.ProofID)
	})
	certen, err := repos.Proofs.CreateProof(ctx, &database.NewCertenAnchorProof{
		ProofArtifactID: artifact.ProofID, AccumTxHash: accumTx, AccountURL: artifact.AccountURL,
		MerkleRoot: h32("root-" + tag), AnchorChain: "base-sepolia", AnchorTxHash: "0x" + tag, AnchorBlockNumber: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repos.ProofArtifacts.SaveValidatorSetSnapshot(ctx, &database.NewValidatorSetSnapshot{
		BlockNumber: 7, ValidatorsJSON: json.RawMessage(`[{"validator_id":"v1","weight":1}]`), ValidatorRoot: h32("vr-" + tag),
		ValidatorCount: 1, TotalWeight: 1, ThresholdWeight: 1, SnapshotHash: h32("snap-" + tag),
		ChainID: "chain-" + tag, ChainName: "records-net",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM validator_set_snapshots WHERE snapshot_id = $1`, snapshot.SnapshotID)
	})
	completion, err := repos.ProofArtifacts.SaveProofCycleCompletion(ctx, &database.NewProofCycleCompletion{ProofID: artifact.ProofID, CycleID: "cycle-" + tag})
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.ProofArtifacts.UpdateProofCycleLevel1(ctx, completion.CompletionID, artifact.ProofID, h32("l1")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	result, err := repos.ProofArtifacts.SaveExternalChainResult(ctx, &database.NewExternalChainResult{
		ProofID: artifact.ProofID, BundleID: h32("b"), OperationID: h32("o"), ChainType: "ethereum", ChainID: "84532",
		ChainName: "base-sepolia", BlockNumber: 9, BlockHash: h32("bh"), BlockTimestamp: now, TransactionHash: h32("tx-" + tag),
		TxFromAddress: h32("f")[:20], StateRoot: h32("s"), TransactionsRoot: h32("t"), ReceiptsRoot: h32("r"),
		ExecutionStatus: 1, GasUsed: 1, SequenceNumber: 0, PreviousResultHash: make([]byte, 32), ResultHash: h32("res-" + tag),
		AnchorProofHash: h32("anchor"), ArtifactJSON: json.RawMessage(`{}`), SnapshotID: &snapshot.SnapshotID,
		IsFinalized: true, ObserverValidatorID: "v1", ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repos.ProofArtifacts.SaveBLSAttestation(ctx, &database.NewBLSAttestation{
		ResultID: result.ResultID, SnapshotID: &snapshot.SnapshotID, ResultHash: result.ResultHash, BundleID: result.BundleID,
		ValidatorID: "v1", ValidatorAddress: h32("v1")[:20], PublicKey: h32("pk"), MessageHash: h32("m"), Signature: h32("sig"),
		Weight: 1, AttestedBlockNumber: 9, AttestedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	request, err := repos.Requests.CreateRequest(ctx, &database.NewProofRequest{
		AccumTxHash: accumTx, RequestType: database.RequestTypeOnDemand, RequesterID: "requester-" + tag,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_requests WHERE request_id = $1`, request.RequestID)
	})

	h := NewProofRecordHandlers(repos, log.New(io.Discard, "", 0))
	get := func(handler http.HandlerFunc, path string) (int, map[string]json.RawMessage) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]json.RawMessage
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}
	field := func(body map[string]json.RawMessage, key string) string { return string(body[key]) }

	code, body := get(h.HandleGetProofCertenProof, "/api/v1/proofs/"+artifact.ProofID.String()+"/certen")
	if code != http.StatusOK || field(body, "proof_hash_verified") != "true" || field(body, "proof_id") != `"`+certen.ProofID.String()+`"` {
		t.Fatalf("certen by artifact: %d %v", code, body)
	}
	code, body = get(h.HandleGetCertenProof, "/api/v1/certen-proofs/"+certen.ProofID.String())
	if code != http.StatusOK || field(body, "proof_hash_verified") != "true" {
		t.Fatalf("certen by id: %d %v", code, body)
	}
	code, _ = get(h.HandleGetCertenProof, "/api/v1/certen-proofs/tx/"+accumTx)
	if code != http.StatusOK {
		t.Fatalf("certen by tx: %d", code)
	}
	if string(body["corrections"]) != "[]" {
		t.Fatalf("an uncorrected proof lists corrections: %s", body["corrections"])
	}
	// A corrected proof carries its correction, which keeps the proof as it was published.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO evidence_corrections (record_type, record_id, reason, previous, corrected, chain_evidence, corrected_by)
		VALUES ('certen_anchor_proof', $1, 'test', '{"proof_hash":"00"}', '{"proof_hash":"11"}', '{}', 'api-test')`,
		certen.ProofID.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM evidence_corrections WHERE record_id = $1`, certen.ProofID.String())
	})
	code, body = get(h.HandleGetCertenProof, "/api/v1/certen-proofs/"+certen.ProofID.String())
	var corrections []database.EvidenceCorrection
	if err := json.Unmarshal(body["corrections"], &corrections); code != http.StatusOK || err != nil || len(corrections) != 1 ||
		string(corrections[0].Previous) != `{"proof_hash":"00"}` {
		t.Fatalf("corrections: %d %s %v", code, body["corrections"], err)
	}
	code, body = get(h.HandleGetProofCycle, "/api/v1/proofs/"+artifact.ProofID.String()+"/cycle")
	if code != http.StatusOK || field(body, "level1_complete") != "true" || field(body, "all_levels_complete") != "false" {
		t.Fatalf("proof cycle: %d %v", code, body)
	}
	code, body = get(h.HandleGetIncompleteProofCycles, "/api/v1/proof-cycles/incomplete?limit=1000")
	if code != http.StatusOK || field(body, "count") == "0" {
		t.Fatalf("incomplete cycles: %d %v", code, body)
	}
	code, body = get(h.HandleGetProofResults, "/api/v1/proofs/"+artifact.ProofID.String()+"/results")
	var results ResultsView
	raw, _ := json.Marshal(body)
	_ = json.Unmarshal(raw, &results)
	if code != http.StatusOK || !results.HashChainValid || len(results.Results) != 1 || len(results.Results[0].Attestations) != 1 || !results.Results[0].MessagesConsistent {
		t.Fatalf("results: %d %+v", code, results)
	}
	code, body = get(h.HandleGetValidatorSet, "/api/v1/validator-sets/"+snapshot.SnapshotID.String())
	if code != http.StatusOK || field(body, "validator_count") != "1" {
		t.Fatalf("validator set: %d %v", code, body)
	}
	code, _ = get(h.HandleGetValidatorSet, "/api/v1/validator-sets/latest/chain-"+tag)
	if code != http.StatusOK {
		t.Fatalf("latest validator set: %d", code)
	}
	code, body = get(h.HandleGetProofRequest, "/api/v1/proof-requests/"+request.RequestID.String())
	if code != http.StatusOK || field(body, "status") != `"pending"` || field(body, "priority") != `"high"` {
		t.Fatalf("proof request: %d %v", code, body)
	}
	code, body = get(h.HandleGetProofRequest, "/api/v1/proof-requests/requester/requester-"+tag)
	if code != http.StatusOK || field(body, "count") != "1" {
		t.Fatalf("requests by requester: %d %v", code, body)
	}

	for path, handler := range map[string]http.HandlerFunc{
		"/api/v1/proofs/" + uuid.NewString() + "/certen": h.HandleGetProofCertenProof,
		"/api/v1/proofs/" + uuid.NewString() + "/cycle":  h.HandleGetProofCycle,
		"/api/v1/validator-sets/" + uuid.NewString():     h.HandleGetValidatorSet,
		"/api/v1/proof-requests/" + uuid.NewString():     h.HandleGetProofRequest,
		"/api/v1/certen-proofs/" + uuid.NewString():      h.HandleGetCertenProof,
	} {
		if code, _ := get(handler, path); code != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", path, code)
		}
	}
	if code, _ := get(h.HandleGetProofCycle, "/api/v1/proofs/not-a-uuid/cycle"); code != http.StatusBadRequest {
		t.Errorf("a malformed proof id answered %d", code)
	}
}
