// Copyright 2025 Certen Protocol
//
// Batch Repository - CRUD operations for anchor batches and batch transactions

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// BatchRepository handles anchor batch operations
type BatchRepository struct {
	client *Client
}

// NewBatchRepository creates a new batch repository
func NewBatchRepository(client *Client) *BatchRepository {
	return &BatchRepository{client: client}
}

// ============================================================================
// BATCH TRANSACTION OPERATIONS
// ============================================================================

// GetTransaction retrieves a transaction by ID
func (r *BatchRepository) GetTransaction(ctx context.Context, txID int64) (*BatchTransaction, error) {
	query := `
		SELECT id, batch_id, accumulate_tx_hash, account_url, tree_index,
			merkle_path, transaction_hash, chained_proof, chained_proof_valid,
			governance_proof, governance_level, governance_valid,
			intent_type, intent_data, created_at
		FROM batch_transactions
		WHERE id = $1`

	tx := &BatchTransaction{}
	err := r.client.QueryRowContext(ctx, query, txID).Scan(
		&tx.ID, &tx.BatchID, &tx.AccumTxHash, &tx.AccountURL, &tx.TreeIndex,
		&tx.MerklePath, &tx.TxHash, &tx.ChainedProof, &tx.ChainedValid,
		&tx.GovProof, &tx.GovLevel, &tx.GovValid,
		&tx.IntentType, &tx.IntentData, &tx.CreatedAt,
	)

	if err == sql.ErrNoRows {
		// F.4 remediation: Return explicit error instead of nil, nil
		return nil, ErrTransactionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return tx, nil
}

// GetTransactionByAccumHash retrieves a transaction by Accumulate tx hash
func (r *BatchRepository) GetTransactionByAccumHash(ctx context.Context, accumTxHash string) (*BatchTransaction, error) {
	query := `
		SELECT id, batch_id, accumulate_tx_hash, account_url, tree_index,
			merkle_path, transaction_hash, chained_proof, chained_proof_valid,
			governance_proof, governance_level, governance_valid,
			intent_type, intent_data, created_at
		FROM batch_transactions
		WHERE accumulate_tx_hash = $1
		ORDER BY created_at DESC
		LIMIT 1`

	tx := &BatchTransaction{}
	err := r.client.QueryRowContext(ctx, query, accumTxHash).Scan(
		&tx.ID, &tx.BatchID, &tx.AccumTxHash, &tx.AccountURL, &tx.TreeIndex,
		&tx.MerklePath, &tx.TxHash, &tx.ChainedProof, &tx.ChainedValid,
		&tx.GovProof, &tx.GovLevel, &tx.GovValid,
		&tx.IntentType, &tx.IntentData, &tx.CreatedAt,
	)

	if err == sql.ErrNoRows {
		// F.4 remediation: Return explicit error instead of nil, nil
		return nil, ErrTransactionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return tx, nil
}

// GetTransactionsInBatch retrieves all transactions in a batch
func (r *BatchRepository) GetTransactionsInBatch(ctx context.Context, batchID uuid.UUID) ([]*BatchTransaction, error) {
	query := `
		SELECT id, batch_id, accumulate_tx_hash, account_url, tree_index,
			merkle_path, transaction_hash, chained_proof, chained_proof_valid,
			governance_proof, governance_level, governance_valid,
			intent_type, intent_data, created_at
		FROM batch_transactions
		WHERE batch_id = $1
		ORDER BY tree_index ASC`

	rows, err := r.client.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	var txs []*BatchTransaction
	for rows.Next() {
		tx := &BatchTransaction{}
		err := rows.Scan(
			&tx.ID, &tx.BatchID, &tx.AccumTxHash, &tx.AccountURL, &tx.TreeIndex,
			&tx.MerklePath, &tx.TxHash, &tx.ChainedProof, &tx.ChainedValid,
			&tx.GovProof, &tx.GovLevel, &tx.GovValid,
			&tx.IntentType, &tx.IntentData, &tx.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		txs = append(txs, tx)
	}

	return txs, rows.Err()
}

// UpdateTransactionProofs updates the proof data for a transaction
func (r *BatchRepository) UpdateTransactionProofs(ctx context.Context, txID int64, chainedProof, govProof json.RawMessage, govLevel GovernanceLevel) error {
	query := `
		UPDATE batch_transactions
		SET chained_proof = $2,
			chained_proof_valid = $3,
			governance_proof = $4,
			governance_level = $5,
			governance_valid = $6
		WHERE id = $1`

	_, err := r.client.ExecContext(ctx, query,
		txID,
		chainedProof, chainedProof != nil,
		govProof, govLevel, govProof != nil,
	)
	if err != nil {
		return fmt.Errorf("failed to update transaction proofs: %w", err)
	}

	return nil
}

// GetNextTreeIndex returns the next tree index for a batch
func (r *BatchRepository) GetNextTreeIndex(ctx context.Context, batchID uuid.UUID) (int, error) {
	query := `SELECT COALESCE(MAX(tree_index), -1) + 1 FROM batch_transactions WHERE batch_id = $1`

	var nextIndex int
	err := r.client.QueryRowContext(ctx, query, batchID).Scan(&nextIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get next tree index: %w", err)
	}

	return nextIndex, nil
}

// UpdateMerklePath updates the merkle path for a transaction
// This is called when a batch is closed and merkle proofs are computed
func (r *BatchRepository) UpdateMerklePath(ctx context.Context, txID int64, merklePath json.RawMessage) error {
	query := `
		UPDATE batch_transactions
		SET merkle_path = $2
		WHERE id = $1`

	result, err := r.client.ExecContext(ctx, query, txID, merklePath)
	if err != nil {
		return fmt.Errorf("failed to update merkle path: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("transaction %d not found", txID)
	}

	return nil
}

// UpdateMerklePathByTreeIndex updates the merkle path for a transaction by batch ID and tree index
func (r *BatchRepository) UpdateMerklePathByTreeIndex(ctx context.Context, batchID uuid.UUID, treeIndex int, merklePath json.RawMessage) error {
	query := `
		UPDATE batch_transactions
		SET merkle_path = $3
		WHERE batch_id = $1 AND tree_index = $2`

	result, err := r.client.ExecContext(ctx, query, batchID, treeIndex, merklePath)
	if err != nil {
		return fmt.Errorf("failed to update merkle path: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("transaction with tree_index %d in batch %s not found", treeIndex, batchID)
	}

	return nil
}
