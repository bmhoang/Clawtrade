package security

import (
	"sync"
	"testing"
)

func TestPermissionMatrix_DefaultDeny(t *testing.T) {
	pm := NewPermissionMatrix()
	if pm.Check("trade", "binance", "BTC/USD") {
		t.Fatal("expected default deny")
	}
}

func TestPermissionMatrix_GrantAndCheck(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.Grant("trade", "binance", "BTC/USD")

	if !pm.Check("trade", "binance", "BTC/USD") {
		t.Fatal("expected granted permission to pass")
	}
	// Different symbol should be denied
	if pm.Check("trade", "binance", "ETH/USD") {
		t.Fatal("expected deny for different symbol")
	}
	// Different exchange should be denied
	if pm.Check("trade", "coinbase", "BTC/USD") {
		t.Fatal("expected deny for different exchange")
	}
	// Different action should be denied
	if pm.Check("withdraw", "binance", "BTC/USD") {
		t.Fatal("expected deny for different action")
	}
}

func TestPermissionMatrix_Revoke(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.Grant("trade", "binance", "BTC/USD")
	pm.Revoke("trade", "binance", "BTC/USD")

	if pm.Check("trade", "binance", "BTC/USD") {
		t.Fatal("expected deny after revoke")
	}
}

func TestPermissionMatrix_GrantAll(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.GrantAll("view_balance")

	if !pm.Check("view_balance", "binance", "BTC/USD") {
		t.Fatal("expected wildcard grant to allow any exchange/symbol")
	}
	if !pm.Check("view_balance", "coinbase", "ETH/USD") {
		t.Fatal("expected wildcard to match different exchange/symbol")
	}
	// Other actions still denied
	if pm.Check("trade", "binance", "BTC/USD") {
		t.Fatal("expected deny for non-granted action")
	}
}

func TestPermissionMatrix_WildcardSymbol(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.Grant("cancel", "binance", "*")

	if !pm.Check("cancel", "binance", "BTC/USD") {
		t.Fatal("expected wildcard symbol to match")
	}
	if !pm.Check("cancel", "binance", "ETH/USD") {
		t.Fatal("expected wildcard symbol to match any symbol")
	}
	if pm.Check("cancel", "coinbase", "BTC/USD") {
		t.Fatal("expected deny for different exchange")
	}
}

func TestPermissionMatrix_ListPermissions(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.Grant("trade", "binance", "BTC/USD")
	pm.Grant("cancel", "coinbase", "ETH/USD")
	pm.GrantAll("view_balance")

	perms := pm.ListPermissions()
	if len(perms) != 3 {
		t.Fatalf("expected 3 permissions, got %d", len(perms))
	}

	// Should be sorted lexicographically by key
	if perms[0].Action != "cancel" {
		t.Errorf("expected first perm action=cancel, got %s", perms[0].Action)
	}
}

func TestPermissionMatrix_MultipleActions(t *testing.T) {
	pm := NewPermissionMatrix()
	pm.Grant("trade", "binance", "BTC/USD")
	pm.Grant("cancel", "binance", "BTC/USD")
	pm.Grant("view_balance", "binance", "BTC/USD")

	for _, action := range []string{"trade", "cancel", "view_balance"} {
		if !pm.Check(action, "binance", "BTC/USD") {
			t.Errorf("expected %s to be allowed", action)
		}
	}
	if pm.Check("withdraw", "binance", "BTC/USD") {
		t.Fatal("withdraw should be denied")
	}
}

func TestPermissionMatrix_ConcurrentAccess(t *testing.T) {
	pm := NewPermissionMatrix()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pm.Grant("trade", "exchange", "SYM")
			pm.Check("trade", "exchange", "SYM")
			pm.ListPermissions()
		}(i)
	}
	wg.Wait()
}

func TestPermissionMatrix_RevokeNonexistent(t *testing.T) {
	pm := NewPermissionMatrix()
	// Should not panic
	pm.Revoke("trade", "binance", "BTC/USD")
	if pm.Check("trade", "binance", "BTC/USD") {
		t.Fatal("expected deny")
	}
}
