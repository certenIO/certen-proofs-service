package database

import (
	"time"

	"github.com/lib/pq"
)

// IntentLifecycleStatus represents the lifecycle state of an intent
type IntentLifecycleStatus string

const (
	IntentLifecycleSubmitted         IntentLifecycleStatus = "submitted"
	IntentLifecyclePendingSignatures IntentLifecycleStatus = "pending_signatures"
	IntentLifecycleAuthorized        IntentLifecycleStatus = "authorized"
	IntentLifecycleInProcess         IntentLifecycleStatus = "in_process"
	// IntentLifecycleSettling: consensus committed and the target-chain write in flight (validator 00000_baseline comment).
	IntentLifecycleSettling IntentLifecycleStatus = "settling"
	IntentLifecycleComplete IntentLifecycleStatus = "complete"
	IntentLifecycleFailed   IntentLifecycleStatus = "failed"
)

// IntentLifecycle represents a row in the intent_lifecycle table
type IntentLifecycle struct {
	ID           int64                 `json:"id"`
	IntentID     string                `json:"intent_id"`
	AccumTxHash  string                `json:"accum_tx_hash"`
	UserID       *string               `json:"user_id,omitempty"`
	Status       IntentLifecycleStatus `json:"status"`
	TargetChain  *string               `json:"target_chain,omitempty"`
	ProofClass   *string               `json:"proof_class,omitempty"`
	ErrorMessage *string               `json:"error_message,omitempty"`
	BlockHeight  *int64                `json:"block_height,omitempty"`
	CycleID      *string               `json:"cycle_id,omitempty"`
	WriteBackTx  *string               `json:"write_back_tx,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	SubmittedAt  *time.Time            `json:"submitted_at,omitempty"`
	AuthorizedAt *time.Time            `json:"authorized_at,omitempty"`
	InProcessAt  *time.Time            `json:"in_process_at,omitempty"`
	CompletedAt  *time.Time            `json:"completed_at,omitempty"`
	FailedAt     *time.Time            `json:"failed_at,omitempty"`

	// The rest of the row the validator writes (RB2-F6). These were not served, so a client could not see how many legs
	// an intent has, its execution mode, how many settled or failed, or that it is settling.
	TargetChains  pq.StringArray `json:"target_chains"`
	LegCount      *int           `json:"leg_count"`
	ExecutionMode *string        `json:"execution_mode"`
	LegsCompleted *int           `json:"legs_completed"`
	LegsFailed    *int           `json:"legs_failed"`
	MemberChains  pq.Int64Array  `json:"member_chains"`
	SettlingAt    *time.Time     `json:"settling_at,omitempty"`

	// Members is each chain member's outcome (validator intent_member_outcomes), served for a single intent. A member
	// chain with no outcome yet is listed with Recorded false, never left out (RB2-F7).
	Members []IntentMemberOutcome `json:"members,omitempty"`
}

// IntentMemberOutcome is one chain member of an intent and what its chain shows (validator migration 00006).
type IntentMemberOutcome struct {
	ChainID      int64      `json:"chain_id"`
	Recorded     bool       `json:"recorded"`
	Settlement   *string    `json:"settlement"`  // settled | reverted | unobserved | none
	ProofCycle   *string    `json:"proof_cycle"` // written | failed
	Legs         *int       `json:"legs"`
	SettlementTx *string    `json:"settlement_tx"`
	WriteBackTx  *string    `json:"write_back_tx"`
	CycleID      *string    `json:"cycle_id"`
	Reason       *string    `json:"reason"`
	RecordedAt   *time.Time `json:"recorded_at"`
}

// IntentLifecycleEnriched extends IntentLifecycle with transaction metadata from batch_transactions
type IntentLifecycleEnriched struct {
	IntentLifecycle
	FromChain   *string `json:"from_chain,omitempty"`
	ToChain     *string `json:"to_chain,omitempty"`
	FromAddress *string `json:"from_address,omitempty"`
	ToAddress   *string `json:"to_address,omitempty"`
	Amount      *string `json:"amount,omitempty"`
	TokenSymbol *string `json:"token_symbol,omitempty"`
	AccountURL  *string `json:"account_url,omitempty"`
}
