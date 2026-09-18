// Copyright 2025 Certen Protocol
//
// Request Repository - CRUD operations for proof requests
// Handles incoming requests for on-cadence (~$0.05) and on-demand (~$0.25) proofs
// Per Whitepaper Section 3.4.2

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RequestRepository handles proof request operations
type RequestRepository struct {
	client *Client
}

// NewRequestRepository creates a new request repository
func NewRequestRepository(client *Client) *RequestRepository {
	return &RequestRepository{client: client}
}

// ============================================================================
// STATUS UPDATE OPERATIONS
// ============================================================================

// UpdateRequestStatus updates the status of a request
func (r *RequestRepository) UpdateRequestStatus(ctx context.Context, requestID uuid.UUID, status RequestStatus, errorMsg string) error {
	var query string
	var args []interface{}

	if errorMsg != "" {
		query = `
			UPDATE proof_requests
			SET status = $2, error_message = $3
			WHERE request_id = $1`
		args = []interface{}{requestID, status, errorMsg}
	} else {
		query = `
			UPDATE proof_requests
			SET status = $2
			WHERE request_id = $1`
		args = []interface{}{requestID, status}
	}

	_, err := r.client.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update request status: %w", err)
	}

	return nil
}

// MarkProcessing marks a request as being processed
func (r *RequestRepository) MarkProcessing(ctx context.Context, requestID uuid.UUID) error {
	query := `
		UPDATE proof_requests
		SET status = 'processing', processed_at = $2
		WHERE request_id = $1`

	_, err := r.client.ExecContext(ctx, query, requestID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to mark request processing: %w", err)
	}

	return nil
}

// MarkCompleted marks a request as completed and assigns the proof ID
func (r *RequestRepository) MarkCompleted(ctx context.Context, requestID uuid.UUID, proofID uuid.UUID) error {
	query := `
		UPDATE proof_requests
		SET status = 'completed', proof_id = $2, completed_at = $3
		WHERE request_id = $1`

	_, err := r.client.ExecContext(ctx, query, requestID, proofID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to mark request completed: %w", err)
	}

	return nil
}

// MarkFailed marks a request as failed
func (r *RequestRepository) MarkFailed(ctx context.Context, requestID uuid.UUID, errorMsg string) error {
	query := `
		UPDATE proof_requests
		SET status = 'failed', error_message = $2, retry_count = retry_count + 1
		WHERE request_id = $1`

	_, err := r.client.ExecContext(ctx, query, requestID, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to mark request failed: %w", err)
	}

	return nil
}

// ResetToRetry resets a failed request to pending for retry
func (r *RequestRepository) ResetToRetry(ctx context.Context, requestID uuid.UUID) error {
	query := `
		UPDATE proof_requests
		SET status = 'pending', processed_at = NULL, error_message = NULL
		WHERE request_id = $1 AND status = 'failed'`

	result, err := r.client.ExecContext(ctx, query, requestID)
	if err != nil {
		return fmt.Errorf("failed to reset request: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("request not found or not in failed status")
	}

	return nil
}

// ============================================================================
// QUERY/STATS OPERATIONS
// ============================================================================

// CountPendingRequests returns the number of pending requests
func (r *RequestRepository) CountPendingRequests(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM proof_requests WHERE status = 'pending'`

	var count int64
	err := r.client.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending requests: %w", err)
	}

	return count, nil
}

// CountByStatus returns the count of requests by status
func (r *RequestRepository) CountByStatus(ctx context.Context, status RequestStatus) (int64, error) {
	query := `SELECT COUNT(*) FROM proof_requests WHERE status = $1`

	var count int64
	err := r.client.QueryRowContext(ctx, query, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count requests by status: %w", err)
	}

	return count, nil
}
