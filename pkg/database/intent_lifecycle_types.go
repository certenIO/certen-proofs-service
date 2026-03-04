package database

import "time"

// IntentLifecycleStatus represents the lifecycle state of an intent
type IntentLifecycleStatus string

const (
	IntentLifecycleSubmitted        IntentLifecycleStatus = "submitted"
	IntentLifecyclePendingSignatures IntentLifecycleStatus = "pending_signatures"
	IntentLifecycleAuthorized       IntentLifecycleStatus = "authorized"
	IntentLifecycleInProcess        IntentLifecycleStatus = "in_process"
	IntentLifecycleComplete         IntentLifecycleStatus = "complete"
	IntentLifecycleFailed           IntentLifecycleStatus = "failed"
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
