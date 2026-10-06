package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IntentLifecycleRepository handles read-only queries for intent lifecycle tracking
type IntentLifecycleRepository struct {
	client *Client
}

// NewIntentLifecycleRepository creates a new intent lifecycle repository
func NewIntentLifecycleRepository(client *Client) *IntentLifecycleRepository {
	return &IntentLifecycleRepository{client: client}
}

// lifecycleColumns is every intent_lifecycle column the service serves, in scanLifecycle's order.
const lifecycleColumns = `il.id, il.intent_id, il.accum_tx_hash, il.user_id, il.status,
		il.target_chain, il.proof_class, il.error_message, il.block_height,
		il.cycle_id, il.write_back_tx,
		il.created_at, il.updated_at, il.submitted_at, il.authorized_at,
		il.in_process_at, il.completed_at, il.failed_at,
		il.target_chains, il.leg_count, il.execution_mode, il.legs_completed, il.legs_failed,
		il.member_chains, il.settling_at, il.failure_class`

// lifecycleDest is the scan destinations for lifecycleColumns.
func lifecycleDest(lc *IntentLifecycle) []interface{} {
	return []interface{}{
		&lc.ID, &lc.IntentID, &lc.AccumTxHash, &lc.UserID, &lc.Status,
		&lc.TargetChain, &lc.ProofClass, &lc.ErrorMessage, &lc.BlockHeight,
		&lc.CycleID, &lc.WriteBackTx,
		&lc.CreatedAt, &lc.UpdatedAt, &lc.SubmittedAt, &lc.AuthorizedAt,
		&lc.InProcessAt, &lc.CompletedAt, &lc.FailedAt,
		&lc.TargetChains, &lc.LegCount, &lc.ExecutionMode, &lc.LegsCompleted, &lc.LegsFailed,
		&lc.MemberChains, &lc.SettlingAt, &lc.FailureClass,
	}
}

// The single-intent reads, as complete statements (the schema test prepares every statement the service can run).
const (
	lifecycleByIntentID = `SELECT ` + lifecycleColumns + ` FROM intent_lifecycle il WHERE il.intent_id = $1`
	lifecycleByTxHash   = `SELECT ` + lifecycleColumns + ` FROM intent_lifecycle il WHERE il.accum_tx_hash = $1`
)

// getOne reads one lifecycle row and its members.
func (r *IntentLifecycleRepository) getOne(ctx context.Context, query string, arg interface{}) (*IntentLifecycle, error) {
	lc := &IntentLifecycle{}
	err := r.client.QueryRowContext(ctx, query, arg).Scan(lifecycleDest(lc)...)
	if err == sql.ErrNoRows {
		return nil, ErrIntentLifecycleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get intent lifecycle: %w", err)
	}
	members, err := r.members(ctx, lc)
	if err != nil {
		return nil, err
	}
	lc.Members = members
	return lc, nil
}

// members is every chain member of the intent: each chain in member_chains with its outcome, or Recorded false when the
// validator has recorded none yet, plus any recorded outcome for a chain member_chains does not list.
func (r *IntentLifecycleRepository) members(ctx context.Context, lc *IntentLifecycle) ([]IntentMemberOutcome, error) {
	rows, err := r.client.QueryContext(ctx, `
		SELECT m.chain_id, m.settlement, m.proof_cycle, m.legs, m.settlement_tx, m.write_back_tx, m.cycle_id, m.reason, m.recorded_at,
		       m.effects_proven,
		       (SELECT p.proof_id::text FROM proof_artifacts p
		         WHERE p.accum_tx_hash = $2 AND m.cycle_id IS NOT NULL AND p.artifact_json->>'cycle_id' = m.cycle_id
		         ORDER BY p.created_at DESC LIMIT 1)
		FROM intent_member_outcomes m WHERE m.intent_id = $1 ORDER BY m.chain_id`, lc.IntentID, lc.AccumTxHash)
	if err != nil {
		return nil, fmt.Errorf("query intent member outcomes: %w", err)
	}
	defer rows.Close()
	recorded := map[int64]IntentMemberOutcome{}
	var order []int64
	for rows.Next() {
		m := IntentMemberOutcome{Recorded: true}
		if err := rows.Scan(&m.ChainID, &m.Settlement, &m.ProofCycle, &m.Legs, &m.SettlementTx, &m.WriteBackTx, &m.CycleID, &m.Reason, &m.RecordedAt,
			&m.EffectsProven, &m.ProofID); err != nil {
			return nil, fmt.Errorf("scan intent member outcome: %w", err)
		}
		recorded[m.ChainID] = m
		order = append(order, m.ChainID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intent member outcomes: %w", err)
	}
	var out []IntentMemberOutcome
	seen := map[int64]bool{}
	for _, c := range lc.MemberChains {
		if m, ok := recorded[c]; ok {
			out = append(out, m)
		} else {
			out = append(out, IntentMemberOutcome{ChainID: c, Recorded: false})
		}
		seen[c] = true
	}
	for _, c := range order {
		if !seen[c] {
			out = append(out, recorded[c])
		}
	}
	return out, nil
}

// GetByIntentID retrieves a lifecycle record, with its members, by intent ID.
func (r *IntentLifecycleRepository) GetByIntentID(ctx context.Context, intentID string) (*IntentLifecycle, error) {
	return r.getOne(ctx, lifecycleByIntentID, intentID)
}

// GetByTxHash retrieves a lifecycle record, with its members, by Accumulate transaction hash.
func (r *IntentLifecycleRepository) GetByTxHash(ctx context.Context, txHash string) (*IntentLifecycle, error) {
	return r.getOne(ctx, lifecycleByTxHash, TransactionHashKey(txHash))
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
		       ` + lifecycleColumns + `,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		ORDER BY il.intent_id, bt.id ASC NULLS LAST
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
		       ` + lifecycleColumns + `,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		WHERE il.user_id = $1
		ORDER BY il.intent_id, bt.id ASC NULLS LAST
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
		       ` + lifecycleColumns + `,
		       bt.from_chain, bt.to_chain, bt.from_address, bt.to_address,
		       bt.amount, bt.token_symbol, bt.account_url
		FROM intent_lifecycle il
		LEFT JOIN batch_transactions bt ON bt.intent_id = il.intent_id
		WHERE il.status = $1
		ORDER BY il.intent_id, bt.id ASC NULLS LAST
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
		dest := append(lifecycleDest(&e.IntentLifecycle),
			&e.FromChain, &e.ToChain, &e.FromAddress, &e.ToAddress,
			&e.Amount, &e.TokenSymbol, &e.AccountURL)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan enriched intent lifecycle row: %w", err)
		}
		results = append(results, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enriched intent lifecycle rows: %w", err)
	}

	return results, nil
}

// ============================================================================
// RECORDED-MEMBER FEED
// ============================================================================
//
// An intent on several chains has one proof request and one chain member per chain. The request ends with the first
// member's proof; each later member's outcome - its settlement, its proof cycle, the proof that cycle produced - is
// recorded in intent_member_outcomes, in the same transaction that derives the intent's lifecycle from its members. A
// caller that learns of ends only from the request feed learns nothing when the last member lands, so the request feed
// serves these records beside its requests (server.HandleTerminalRequests).

// MemberPosition is a place in the recorded-member feed: after the member (IntentID, ChainID) recorded at RecordedAt.
// An empty IntentID sorts before every intent at that instant.
type MemberPosition struct {
	RecordedAt time.Time
	IntentID   string
	ChainID    int64
}

// RecordedMember is one chain member's recorded outcome, as the feed serves it.
type RecordedMember struct {
	IntentID    string
	ChainID     int64
	AccumTxHash sql.NullString // the intent's, from its lifecycle row; null when the member has none
	Settlement  string
	ProofCycle  string
	ProofID     sql.NullString // the proof the member's recording cycle produced, as members() names it
	RecordedAt  time.Time
}

// membersRecordedAfter orders by (recorded_at, intent_id, chain_id), the id compared byte-wise (COLLATE "C") so the
// order, and so the cursor, does not depend on the database's collation.
const membersRecordedAfter = `SELECT m.intent_id, m.chain_id, l.accum_tx_hash, m.settlement, m.proof_cycle,
		(SELECT p.proof_id::text FROM proof_artifacts p
		  WHERE p.accum_tx_hash = l.accum_tx_hash AND m.cycle_id IS NOT NULL AND p.artifact_json->>'cycle_id' = m.cycle_id
		  ORDER BY p.created_at DESC LIMIT 1),
		m.recorded_at
	FROM intent_member_outcomes m LEFT JOIN intent_lifecycle l ON l.intent_id = m.intent_id
	WHERE m.recorded_at <= NOW() - make_interval(secs => $1)
	  AND (m.recorded_at, m.intent_id COLLATE "C", m.chain_id) > ($2::timestamptz, $3::text COLLATE "C", $4::bigint)
	ORDER BY m.recorded_at ASC, m.intent_id COLLATE "C" ASC, m.chain_id ASC
	LIMIT $5`

// GetMembersRecordedAfter returns up to limit chain-member outcomes recorded after `after`, in (recorded_at, intent_id,
// chain_id) order, leaving out those recorded within TerminalSettleWindow: recorded_at is the recording transaction's
// start time, so a row can commit after a later-stamped row was served, exactly as a request's completed_at can.
//
// A member is recorded again whenever its outcome changes (recorded_at moves to the new record), so each record of it
// is served once, at the place its latest record holds.
func (r *IntentLifecycleRepository) GetMembersRecordedAfter(ctx context.Context, after MemberPosition, limit int) ([]RecordedMember, error) {
	rows, err := r.client.QueryContext(ctx, membersRecordedAfter,
		TerminalSettleWindow.Seconds(), after.RecordedAt, after.IntentID, after.ChainID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recorded members: %w", err)
	}
	defer rows.Close()
	var out []RecordedMember
	for rows.Next() {
		var m RecordedMember
		if err := rows.Scan(&m.IntentID, &m.ChainID, &m.AccumTxHash, &m.Settlement, &m.ProofCycle, &m.ProofID, &m.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan recorded member: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recorded members: %w", err)
	}
	return out, nil
}
