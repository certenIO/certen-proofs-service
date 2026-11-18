package server

import "testing"

// AGT-F11: a bundle anchor reference that carries a state (anchor_block_unread) is not reported as a present component.
func TestAnAnchorReferenceWithAStateIsNotPresent(t *testing.T) {
	if anchorReferencePresent(map[string]interface{}{"target_chain": "84532", "anchor_tx_hash": "0x" + "43", "state": "anchor_block_unread"}) {
		t.Fatal("an unread anchor reference was reported present")
	}
	if !anchorReferencePresent(map[string]interface{}{"target_chain": "84532", "anchor_tx_hash": "0x43", "anchor_block_hash": "0x4d", "anchored_at": "2026-11-18T11:30:20Z"}) {
		t.Fatal("a complete anchor reference was reported absent")
	}
	if anchorReferencePresent(nil) {
		t.Fatal("no reference was reported present")
	}
}
