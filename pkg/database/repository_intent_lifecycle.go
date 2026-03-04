package database

import (
	"context"
	"database/sql"
	"fmt"
)

// IntentLifecycleRepository handles read-only queries for intent lifecycle tracking
type IntentLifecycleRepository struct {
	client *Client
}

// NewIntentLifecycleRepository creates a new intent lifecycle repository
func NewIntentLifecycleRepository(client *Client) *IntentLifecycleRepository {
	return &IntentLifecycleRepository{client: client}
}

// GetByIntentID retrieves a lifecycle record by intent ID
func (r *IntentLifecycleRepository) GetByIntentID(ctx context.Context, intentID string) (*IntentLifecycle, error) {
	query := `
		SELECT id, intent_id, accum_tx_hash, user_id, status, target_chain, proof_class,
		       error_message, block_height, cycle_id, write_back_tx,
		       created_at, updated_at, submitted_at, authorized_at,
		       in_process_at, completed_at, failed_at
		FROM intent_lifecycle
		WHERE intent_id = $1
	`

	lc := &IntentLifecycle{}
	err := r.client.QueryRowContext(ctx, query, intentID).Scan(
		&lc.ID, &lc.IntentID, &lc.AccumTxHash, &lc.UserID, &lc.Status,
		&lc.TargetChain, &lc.ProofClass, &lc.ErrorMessage, &lc.BlockHeight,
		&lc.CycleID, &lc.WriteBackTx,
		&lc.CreatedAt, &lc.UpdatedAt, &lc.SubmittedAt, &lc.AuthorizedAt,
		&lc.InProcessAt, &lc.CompletedAt, &lc.FailedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrIntentLifecycleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get intent lifecycle by id: %w", err)
	}
	return lc, nil
}

// GetByTxHash retrieves a lifecycle record by Accumulate transaction hash
func (r *IntentLifecycleRepository) GetByTxHash(ctx context.Context, txHash string) (*IntentLifecycle, error) {
	query := `
		SELECT id, intent_id, accum_tx_hash, user_id, status, target_chain, proof_class,
		       error_message, block_height, cycle_id, write_back_tx,
		       created_at, updated_at, submitted_at, authorized_at,
		       in_process_at, completed_at, failed_at
		FROM intent_lifecycle
		WHERE accum_tx_hash = $1
	`

	lc := &IntentLifecycle{}
	err := r.client.QueryRowContext(ctx, query, txHash).Scan(
		&lc.ID, &lc.IntentID, &lc.AccumTxHash, &lc.UserID, &lc.Status,
		&lc.TargetChain, &lc.ProofClass, &lc.ErrorMessage, &lc.BlockHeight,
		&lc.CycleID, &lc.WriteBackTx,
		&lc.CreatedAt, &lc.UpdatedAt, &lc.SubmittedAt, &lc.AuthorizedAt,
		&lc.InProcessAt, &lc.CompletedAt, &lc.FailedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrIntentLifecycleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get intent lifecycle by tx hash: %w", err)
	}
	return lc, nil
}

// ListRecentEnriched returns recent lifecycle records joined with batch_transactions
func (r *IntentLifecycleRepository) ListRecentEnriched(ctx context.Context, limit int) ([]*IntentLifecycleEnriched, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT DISTINCT ON (il.intent_id)
		       il.id, il.intent_id, il.accum_tx_hash, il.user_id, il.status,
		       il.target_chain, il.proof_class, il.error_message, il.block_height,
		       il.cycle_id, il.write_back_tx,
		       il.created_at, il.updated_at, il.submitted_at, il.authorized_at,
		       il.in_process_at, il.completed_at, il.failed_at,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		ORDER BY il.intent_id, il.created_at DESC
	`

	// Wrap with outer query to apply limit and final ordering
	wrappedQuery := fmt.Sprintf(`SELECT * FROM (%s) sub ORDER BY created_at DESC LIMIT $1`, query)
	return r.scanEnrichedRows(ctx, wrappedQuery, limit)
}

// ListByUserEnriched returns lifecycle records for a user joined with batch_transactions
func (r *IntentLifecycleRepository) ListByUserEnriched(ctx context.Context, userID string, limit int) ([]*IntentLifecycleEnriched, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT DISTINCT ON (il.intent_id)
		       il.id, il.intent_id, il.accum_tx_hash, il.user_id, il.status,
		       il.target_chain, il.proof_class, il.error_message, il.block_height,
		       il.cycle_id, il.write_back_tx,
		       il.created_at, il.updated_at, il.submitted_at, il.authorized_at,
		       il.in_process_at, il.completed_at, il.failed_at,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		WHERE il.user_id = $1
		ORDER BY il.intent_id, il.created_at DESC
	`

	wrappedQuery := fmt.Sprintf(`SELECT * FROM (%s) sub ORDER BY created_at DESC LIMIT $2`, query)
	return r.scanEnrichedRows(ctx, wrappedQuery, userID, limit)
}

// ListByStatus returns lifecycle records filtered by status
func (r *IntentLifecycleRepository) ListByStatus(ctx context.Context, status IntentLifecycleStatus, limit int) ([]*IntentLifecycleEnriched, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT DISTINCT ON (il.intent_id)
		       il.id, il.intent_id, il.accum_tx_hash, il.user_id, il.status,
		       il.target_chain, il.proof_class, il.error_message, il.block_height,
		       il.cycle_id, il.write_back_tx,
		       il.created_at, il.updated_at, il.submitted_at, il.authorized_at,
		       il.in_process_at, il.completed_at, il.failed_at,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		WHERE il.status = $1
		ORDER BY il.intent_id, il.created_at DESC
	`

	wrappedQuery := fmt.Sprintf(`SELECT * FROM (%s) sub ORDER BY created_at DESC LIMIT $2`, query)
	return r.scanEnrichedRows(ctx, wrappedQuery, string(status), limit)
}

// scanEnrichedRows scans rows from a joined lifecycle + batch_transactions query
func (r *IntentLifecycleRepository) scanEnrichedRows(ctx context.Context, query string, args ...interface{}) ([]*IntentLifecycleEnriched, error) {
	rows, err := r.client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query enriched intent lifecycles: %w", err)
	}
	defer rows.Close()

	var results []*IntentLifecycleEnriched
	for rows.Next() {
		e := &IntentLifecycleEnriched{}
		if err := rows.Scan(
			&e.ID, &e.IntentID, &e.AccumTxHash, &e.UserID, &e.Status,
			&e.TargetChain, &e.ProofClass, &e.ErrorMessage, &e.BlockHeight,
			&e.CycleID, &e.WriteBackTx,
			&e.CreatedAt, &e.UpdatedAt, &e.SubmittedAt, &e.AuthorizedAt,
			&e.InProcessAt, &e.CompletedAt, &e.FailedAt,
			&e.FromChain, &e.ToChain, &e.FromAddress, &e.ToAddress,
			&e.Amount, &e.TokenSymbol, &e.AccountURL,
		); err != nil {
			return nil, fmt.Errorf("scan enriched intent lifecycle row: %w", err)
		}
		results = append(results, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enriched intent lifecycle rows: %w", err)
	}

	return results, nil
}
