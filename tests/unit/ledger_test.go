package unit_test

import (
	"testing"

	"ledger-service/shard/ledger"
)

// TestCreateAccountAndGetBalance verifies basic account creation and retrieval.
func TestCreateAccountAndGetBalance(t *testing.T) {
	l := ledger.NewLedger()

	if err := l.CreateAccount("alice", 1000); err != nil {
		t.Fatalf("CreateAccount alice: %v", err)
	}

	bal, ok := l.GetBalance("alice")
	if !ok {
		t.Fatal("alice should exist")
	}
	if bal != 1000 {
		t.Errorf("alice balance = %d, want 1000", bal)
	}
}

// TestCreateAccountNegativeBalanceFails ensures negative opening balances are rejected.
func TestCreateAccountNegativeBalanceFails(t *testing.T) {
	l := ledger.NewLedger()
	err := l.CreateAccount("bad", -1)
	if err == nil {
		t.Error("expected error for negative opening balance, got nil")
	}
}

// TestCreateDuplicateAccountFails ensures we can't create the same account twice.
func TestCreateDuplicateAccountFails(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)
	err := l.CreateAccount("alice", 500)
	if err == nil {
		t.Error("expected error creating duplicate account, got nil")
	}
}

// TestApplyTransferSuccess verifies a valid transfer updates both balances correctly.
func TestApplyTransferSuccess(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 1000)
	_ = l.CreateAccount("bob", 500)

	totalBefore := l.TotalBalance()

	if err := l.ApplyTransfer("alice", "bob", 300); err != nil {
		t.Fatalf("ApplyTransfer: %v", err)
	}

	aliceBal, _ := l.GetBalance("alice")
	bobBal, _ := l.GetBalance("bob")

	if aliceBal != 700 {
		t.Errorf("alice balance = %d, want 700", aliceBal)
	}
	if bobBal != 800 {
		t.Errorf("bob balance = %d, want 800", bobBal)
	}

	// Conservation of money invariant
	totalAfter := l.TotalBalance()
	if totalBefore != totalAfter {
		t.Errorf("total balance changed: before=%d after=%d (CONSERVATION VIOLATED)", totalBefore, totalAfter)
	}
}

// TestNoNegativeBalances is the critical invariant test.
// The system MUST NEVER allow a balance to go negative.
func TestNoNegativeBalances(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 100)
	_ = l.CreateAccount("bob", 0)

	err := l.ApplyTransfer("alice", "bob", 200) // alice only has 100
	if err == nil {
		t.Fatal("expected error for overdraft, got nil — NEGATIVE BALANCE INVARIANT VIOLATED")
	}

	// Alice's balance must be unchanged
	aliceBal, _ := l.GetBalance("alice")
	if aliceBal != 100 {
		t.Errorf("alice balance changed after rejected transfer: got %d, want 100", aliceBal)
	}
}

// TestApplyTransferZeroAmountFails ensures zero-amount transfers are rejected.
func TestApplyTransferZeroAmountFails(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)
	_ = l.CreateAccount("bob", 500)

	err := l.ApplyTransfer("alice", "bob", 0)
	if err == nil {
		t.Error("expected error for zero-amount transfer, got nil")
	}
}

// TestApplyTransferUnknownSource fails when source account doesn't exist.
func TestApplyTransferUnknownSource(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("bob", 500)

	err := l.ApplyTransfer("ghost", "bob", 100)
	if err == nil {
		t.Error("expected error for unknown source account, got nil")
	}
}

// TestApplyTransferUnknownDest fails when destination account doesn't exist.
func TestApplyTransferUnknownDest(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)

	err := l.ApplyTransfer("alice", "ghost", 100)
	if err == nil {
		t.Error("expected error for unknown destination account, got nil")
	}
}

// TestConservationOfMoneyMultipleTransfers verifies conservation holds across many transfers.
func TestConservationOfMoneyMultipleTransfers(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 1000)
	_ = l.CreateAccount("bob", 1000)
	_ = l.CreateAccount("charlie", 1000)

	totalBefore := l.TotalBalance() // 3000

	transfers := []struct {
		from, to string
		amount   int64
	}{
		{"alice", "bob", 100},
		{"bob", "charlie", 200},
		{"charlie", "alice", 50},
		{"alice", "charlie", 300},
		{"bob", "alice", 150},
	}

	for _, tr := range transfers {
		if err := l.ApplyTransfer(tr.from, tr.to, tr.amount); err != nil {
			t.Fatalf("transfer %s→%s %d: %v", tr.from, tr.to, tr.amount, err)
		}
	}

	totalAfter := l.TotalBalance()
	if totalBefore != totalAfter {
		t.Errorf("conservation violated: before=%d after=%d", totalBefore, totalAfter)
	}
}

// TestValidateDebitDoesNotChangeBalance verifies validation is side-effect-free.
func TestValidateDebitDoesNotChangeBalance(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)

	_ = l.ValidateDebit("alice", 200)

	bal, _ := l.GetBalance("alice")
	if bal != 500 {
		t.Errorf("ValidateDebit changed balance: got %d, want 500", bal)
	}
}

// TestRollbackDebit restores balance after an aborted cross-shard debit.
func TestRollbackDebit(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)

	_ = l.ApplyDebit("alice", 200) // simulate prepared debit

	bal, _ := l.GetBalance("alice")
	if bal != 300 {
		t.Fatalf("after debit: got %d, want 300", bal)
	}

	_ = l.RollbackDebit("alice", 200) // abort — restore the funds

	bal, _ = l.GetBalance("alice")
	if bal != 500 {
		t.Errorf("after rollback: got %d, want 500", bal)
	}
}

// TestSnapshotIsIsolated verifies that snapshot returns a copy, not a live reference.
func TestSnapshotIsIsolated(t *testing.T) {
	l := ledger.NewLedger()
	_ = l.CreateAccount("alice", 500)

	snap := l.Snapshot()
	snap["alice"] = 9999 // modify the copy

	bal, _ := l.GetBalance("alice")
	if bal != 500 {
		t.Errorf("Snapshot leaked a reference: alice balance = %d, want 500", bal)
	}
}

// TestNewLedgerWithAccountsNegativeFails verifies NewLedgerWithAccounts rejects negatives.
func TestNewLedgerWithAccountsNegativeFails(t *testing.T) {
	_, err := ledger.NewLedgerWithAccounts(map[string]int64{
		"alice": 100,
		"bob":   -50,
	})
	if err == nil {
		t.Error("expected error for negative initial balance, got nil")
	}
}
