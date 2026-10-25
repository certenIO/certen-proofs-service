// Copyright 2026 Certen Protocol
//
// The `contractIdentity` member of a portable proof v2 document (V2_SPEC_S8 Q2 item 3), composed at request time from the validators'
// insert-only proof_contract_identity row exactly as `finality` is composed from the addendum rows. It is evidence, never a verdict: this
// file checks the row is internally consistent (sizes, the stored sha256 of the container) and writes it in the one shape the verifiers read
// (proofverify parseCIMember, the TypeScript verifier): a verifier holding the member judges the container offline.

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// ContractIdentityFormat is the format string of the `contractIdentity` member.
const ContractIdentityFormat = "certen-contract-identity/1"

// MaxContractIdentityBytes is database.MaxContractIdentityBytes (see there for the drift pins).
const MaxContractIdentityBytes = database.MaxContractIdentityBytes

// Named refusals of a composition that cannot be served.
const (
	ContractIdentityMemberInDocument = "contract_identity_member_stored_in_document"
	ContractIdentityRowInvalid       = "contract_identity_row_invalid"
)

type contractIdentityStore interface {
	GetContractIdentity(ctx context.Context, proofID uuid.UUID) (*database.ContractIdentityRow, error)
}

type contractIdentityLegJSON struct {
	Chain       int64  `json:"chain"`
	BlockNumber int64  `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
}

type contractIdentityMemberJSON struct {
	Format   string                  `json:"format"`
	Leg      contractIdentityLegJSON `json:"leg"`
	Evidence string                  `json:"evidence"`
	Digest   string                  `json:"digest"`
}

// ComposeContractIdentity builds the member of a stored row: {"format","leg":{"chain","blockNumber","blockHash"},"evidence":"0x..","digest":"0x.."}
// with lowercase hex. A row that contradicts itself is the named contract_identity_row_invalid, never a repaired member.
func ComposeContractIdentity(row *database.ContractIdentityRow) (json.RawMessage, error) {
	switch {
	case row == nil:
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", "no row")
	case row.Chain <= 0 || row.BlockNumber < 0:
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", fmt.Sprintf("chain %d block %d", row.Chain, row.BlockNumber))
	case len(row.BlockHash) != 32 || len(row.Digest) != 32 || len(row.EvidenceSHA256) != 32:
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", "block_hash, digest and evidence_sha256 must be 32 bytes")
	case len(row.Evidence) == 0:
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", "empty evidence")
	case int64(len(row.Evidence)) > MaxContractIdentityBytes:
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", fmt.Sprintf("evidence is %d bytes, the bound is %d", len(row.Evidence), MaxContractIdentityBytes))
	}
	if sum := sha256.Sum256(row.Evidence); hex.EncodeToString(sum[:]) != hex.EncodeToString(row.EvidenceSHA256) {
		return nil, finalityErr(ContractIdentityRowInvalid, "reason", "evidence_sha256 is not the sha256 of the evidence", "proof", row.ProofID.String(), "seq", seqStr(row.Seq))
	}
	return json.Marshal(contractIdentityMemberJSON{
		Format:   ContractIdentityFormat,
		Leg:      contractIdentityLegJSON{Chain: row.Chain, BlockNumber: row.BlockNumber, BlockHash: "0x" + hex.EncodeToString(row.BlockHash)},
		Evidence: "0x" + hex.EncodeToString(row.Evidence),
		Digest:   "0x" + hex.EncodeToString(row.Digest),
	})
}
