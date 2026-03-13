package adapter

import "testing"

func TestNormalizerParseBinance(t *testing.T) {
	sp := ParseSymbol("BTCUSDT")
	if sp.Base != "BTC" || sp.Quote != "USDT" {
		t.Errorf("expected BTC-USDT, got %s-%s", sp.Base, sp.Quote)
	}
}

func TestNormalizerParseBybit(t *testing.T) {
	sp := ParseSymbol("BTC-USDT")
	if sp.Base != "BTC" || sp.Quote != "USDT" {
		t.Errorf("expected BTC-USDT, got %s-%s", sp.Base, sp.Quote)
	}
}

func TestNormalizerParseGeneric(t *testing.T) {
	sp := ParseSymbol("BTC/USDT")
	if sp.Base != "BTC" || sp.Quote != "USDT" {
		t.Errorf("expected BTC-USDT, got %s-%s", sp.Base, sp.Quote)
	}
}

func TestNormalizerParseCaseInsensitive(t *testing.T) {
	sp := ParseSymbol("eth-usdt")
	if sp.Base != "ETH" || sp.Quote != "USDT" {
		t.Errorf("expected ETH-USDT, got %s-%s", sp.Base, sp.Quote)
	}
}

func TestNormalizerString(t *testing.T) {
	sp := ParseSymbol("BTCUSDT")
	if sp.String() != "BTC-USDT" {
		t.Errorf("expected BTC-USDT, got %s", sp.String())
	}
}

func TestNormalizerForBinance(t *testing.T) {
	sp := ParseSymbol("BTC-USDT")
	if sp.ForBinance() != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", sp.ForBinance())
	}
}

func TestNormalizerForBybit(t *testing.T) {
	sp := ParseSymbol("BTCUSDT")
	if sp.ForBybit() != "BTC-USDT" {
		t.Errorf("expected BTC-USDT, got %s", sp.ForBybit())
	}
}

func TestNormalizerForOKX(t *testing.T) {
	sp := ParseSymbol("BTCUSDT")
	if sp.ForOKX() != "BTC-USDT" {
		t.Errorf("expected BTC-USDT, got %s", sp.ForOKX())
	}
}

func TestNormalizerForGeneric(t *testing.T) {
	sp := ParseSymbol("BTC-USDT")
	if sp.ForGeneric() != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", sp.ForGeneric())
	}
}

func TestNormalizerVariousPairs(t *testing.T) {
	cases := []struct {
		input string
		base  string
		quote string
	}{
		{"ETHUSDT", "ETH", "USDT"},
		{"ETH/BTC", "ETH", "BTC"},
		{"SOL-USDC", "SOL", "USDC"},
		{"BNBBUSD", "BNB", "BUSD"},
		{"DOGEUSDT", "DOGE", "USDT"},
		{"AVAX/USDT", "AVAX", "USDT"},
		{"xrp-usdt", "XRP", "USDT"},
		{"ADAEUR", "ADA", "EUR"},
		{"BTCUSD", "BTC", "USD"},
	}
	for _, tc := range cases {
		sp := ParseSymbol(tc.input)
		if sp.Base != tc.base || sp.Quote != tc.quote {
			t.Errorf("ParseSymbol(%q): expected %s-%s, got %s-%s",
				tc.input, tc.base, tc.quote, sp.Base, sp.Quote)
		}
	}
}

func TestNormalizerRoundTrip(t *testing.T) {
	// Parse from Binance, convert to Bybit, re-parse, convert to Binance
	sp1 := ParseSymbol("ETHUSDT")
	bybitFmt := sp1.ForBybit()
	sp2 := ParseSymbol(bybitFmt)
	binanceFmt := sp2.ForBinance()
	if binanceFmt != "ETHUSDT" {
		t.Errorf("round-trip failed: expected ETHUSDT, got %s", binanceFmt)
	}
}

func TestNormalizerWithWhitespace(t *testing.T) {
	sp := ParseSymbol("  BTC-USDT  ")
	if sp.Base != "BTC" || sp.Quote != "USDT" {
		t.Errorf("expected BTC-USDT, got %s-%s", sp.Base, sp.Quote)
	}
}
