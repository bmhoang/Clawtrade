package mql5

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

func TestNewBridge(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if b == nil {
		t.Fatal("expected non-nil bridge")
	}
	if b.address != "127.0.0.1:0" {
		t.Fatalf("expected address 127.0.0.1:0, got %s", b.address)
	}
	if b.prices == nil {
		t.Fatal("expected prices map initialized")
	}
	if b.handlers == nil {
		t.Fatal("expected handlers map initialized")
	}
}

func TestStartStop(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	addr := b.Address()
	if addr == "" || addr == "127.0.0.1:0" {
		t.Fatalf("expected resolved address, got %s", addr)
	}

	if err := b.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop should not panic.
	if err := b.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestIsConnected(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	if b.IsConnected() {
		t.Fatal("expected not connected before any client connects")
	}

	// Connect a client.
	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Give the accept loop time to process.
	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	if !b.IsConnected() {
		t.Fatal("expected connected after client connects")
	}
}

func TestSendMessage(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	msg := Message{
		Type:   MsgTypeCommand,
		ID:     "test-1",
		Symbol: "EURUSD",
	}

	if err := b.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Read from client side.
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var received Message
	if err := json.Unmarshal(buf[:n-1], &received); err != nil { // -1 for newline
		t.Fatalf("unmarshal failed: %v", err)
	}

	if received.Type != MsgTypeCommand {
		t.Fatalf("expected type %s, got %s", MsgTypeCommand, received.Type)
	}
	if received.ID != "test-1" {
		t.Fatalf("expected id test-1, got %s", received.ID)
	}
	if received.Symbol != "EURUSD" {
		t.Fatalf("expected symbol EURUSD, got %s", received.Symbol)
	}
	if received.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestSendNotConnected(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	err := b.Send(Message{Type: MsgTypeCommand})
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestGetLatestPrice(t *testing.T) {
	b := NewBridge("127.0.0.1:0")

	// No price initially.
	_, ok := b.GetLatestPrice("EURUSD")
	if ok {
		t.Fatal("expected no price initially")
	}

	// Manually set a price.
	b.mu.Lock()
	b.prices["EURUSD"] = PriceData{
		Symbol: "EURUSD",
		Bid:    1.1000,
		Ask:    1.1002,
		Last:   1.1001,
		Volume: 1000,
		Time:   time.Now().UnixMilli(),
	}
	b.mu.Unlock()

	p, ok := b.GetLatestPrice("EURUSD")
	if !ok {
		t.Fatal("expected price to be found")
	}
	if p.Bid != 1.1000 {
		t.Fatalf("expected bid 1.1, got %f", p.Bid)
	}
	if p.Ask != 1.1002 {
		t.Fatalf("expected ask 1.1002, got %f", p.Ask)
	}
}

func TestOnMessageHandler(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	received := make(chan Message, 1)
	b.OnMessage(MsgTypeHeartbeat, func(msg Message) error {
		received <- msg
		return nil
	})

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	// Send a heartbeat message from "MetaTrader" side.
	hb := Message{
		Type:      MsgTypeHeartbeat,
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(hb)
	data = append(data, '\n')
	conn.Write(data)

	select {
	case msg := <-received:
		if msg.Type != MsgTypeHeartbeat {
			t.Fatalf("expected heartbeat, got %s", msg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for heartbeat handler")
	}
}

func TestPriceUpdateFromConnection(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	// Send a price message from "MetaTrader".
	pd := PriceData{
		Symbol: "GBPUSD",
		Bid:    1.2500,
		Ask:    1.2502,
		Last:   1.2501,
		Volume: 500,
		Time:   time.Now().UnixMilli(),
	}
	pdData, _ := json.Marshal(pd)

	msg := Message{
		Type:      MsgTypePrice,
		Symbol:    "GBPUSD",
		Data:      json.RawMessage(pdData),
		Timestamp: time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	conn.Write(data)

	// Wait for price to be cached.
	waitFor(t, func() bool {
		_, ok := b.GetLatestPrice("GBPUSD")
		return ok
	}, 2*time.Second)

	p, ok := b.GetLatestPrice("GBPUSD")
	if !ok {
		t.Fatal("expected price to be cached")
	}
	if p.Bid != 1.2500 {
		t.Fatalf("expected bid 1.25, got %f", p.Bid)
	}
}

func TestSendOrder(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	cmd := OrderCommand{
		Action: "buy",
		Symbol: "EURUSD",
		Volume: 0.1,
	}

	if err := b.SendOrder(cmd); err != nil {
		t.Fatalf("SendOrder failed: %v", err)
	}

	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var received Message
	if err := json.Unmarshal(buf[:n-1], &received); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if received.Type != MsgTypeCommand {
		t.Fatalf("expected type command, got %s", received.Type)
	}
	if received.Symbol != "EURUSD" {
		t.Fatalf("expected symbol EURUSD, got %s", received.Symbol)
	}

	var order OrderCommand
	if err := json.Unmarshal(received.Data, &order); err != nil {
		t.Fatalf("unmarshal order data failed: %v", err)
	}
	if order.Action != "buy" {
		t.Fatalf("expected action buy, got %s", order.Action)
	}
	if order.Volume != 0.1 {
		t.Fatalf("expected volume 0.1, got %f", order.Volume)
	}
}

// Compile-time check that MQL5Adapter implements TradingAdapter.
var _ adapter.TradingAdapter = (*MQL5Adapter)(nil)

func TestMQL5AdapterName(t *testing.T) {
	a := NewMQL5Adapter("127.0.0.1:0")
	if a.Name() != "mql5" {
		t.Fatalf("expected name mql5, got %s", a.Name())
	}
}

func TestMQL5AdapterCapabilities(t *testing.T) {
	a := NewMQL5Adapter("127.0.0.1:0")
	caps := a.Capabilities()

	if caps.Name != "mql5" {
		t.Fatalf("expected caps name mql5, got %s", caps.Name)
	}
	if caps.WebSocket {
		t.Fatal("expected websocket false")
	}
	if !caps.Margin {
		t.Fatal("expected margin true")
	}
	if caps.Futures {
		t.Fatal("expected futures false")
	}
	if len(caps.OrderTypes) != 3 {
		t.Fatalf("expected 3 order types, got %d", len(caps.OrderTypes))
	}
}

func TestMQL5AdapterConnectDisconnect(t *testing.T) {
	a := NewMQL5Adapter("127.0.0.1:0")

	if a.IsConnected() {
		t.Fatal("expected not connected before Connect")
	}

	if err := a.Connect(nil); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Not connected until a client dials in; IsConnected should still be false.
	if a.IsConnected() {
		t.Fatal("expected not connected (no MetaTrader client)")
	}

	if err := a.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
}

func TestOnConnectDisconnectCallbacks(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	connectCalled := make(chan struct{}, 1)
	disconnectCalled := make(chan struct{}, 1)

	b.OnConnect(func() {
		connectCalled <- struct{}{}
	})
	b.OnDisconnect(func() {
		disconnectCalled <- struct{}{}
	})

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	select {
	case <-connectCalled:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnConnect callback")
	}

	conn.Close()

	select {
	case <-disconnectCalled:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnDisconnect callback")
	}
}

func TestConcurrentPriceUpdates(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	var wg sync.WaitGroup
	symbols := []string{"EURUSD", "GBPUSD", "USDJPY", "AUDUSD"}

	// Concurrently send price updates from multiple goroutines.
	for i, sym := range symbols {
		wg.Add(1)
		go func(symbol string, idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				pd := PriceData{
					Symbol: symbol,
					Bid:    1.0 + float64(idx)*0.1 + float64(j)*0.001,
					Ask:    1.0 + float64(idx)*0.1 + float64(j)*0.001 + 0.0002,
					Last:   1.0 + float64(idx)*0.1 + float64(j)*0.001 + 0.0001,
					Volume: float64(j * 100),
					Time:   time.Now().UnixMilli(),
				}
				pdData, _ := json.Marshal(pd)
				msg := Message{
					Type:      MsgTypePrice,
					Symbol:    symbol,
					Data:      json.RawMessage(pdData),
					Timestamp: time.Now().UnixMilli(),
				}
				data, _ := json.Marshal(msg)
				data = append(data, '\n')
				conn.Write(data)
			}
		}(sym, i)
	}

	// Concurrently read prices.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, sym := range symbols {
				b.GetLatestPrice(sym)
			}
		}()
	}

	wg.Wait()

	// Wait for all messages to be processed.
	time.Sleep(200 * time.Millisecond)

	// All symbols should have a cached price.
	for _, sym := range symbols {
		p, ok := b.GetLatestPrice(sym)
		if !ok {
			t.Fatalf("expected price for %s", sym)
		}
		if p.Symbol != sym {
			t.Fatalf("expected symbol %s, got %s", sym, p.Symbol)
		}
	}
}

func TestMultipleMessageTypes(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	if err := b.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer b.Stop()

	orderReceived := make(chan Message, 1)
	accountReceived := make(chan Message, 1)

	b.OnMessage(MsgTypeOrder, func(msg Message) error {
		orderReceived <- msg
		return nil
	})
	b.OnMessage(MsgTypeAccount, func(msg Message) error {
		accountReceived <- msg
		return nil
	})

	conn, err := net.Dial("tcp", b.Address())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	waitFor(t, func() bool { return b.IsConnected() }, 2*time.Second)

	// Send order message.
	orderMsg := Message{Type: MsgTypeOrder, ID: "ord-1", Timestamp: time.Now().UnixMilli()}
	data, _ := json.Marshal(orderMsg)
	conn.Write(append(data, '\n'))

	// Send account message.
	acctMsg := Message{Type: MsgTypeAccount, ID: "acct-1", Timestamp: time.Now().UnixMilli()}
	data, _ = json.Marshal(acctMsg)
	conn.Write(append(data, '\n'))

	select {
	case msg := <-orderReceived:
		if msg.ID != "ord-1" {
			t.Fatalf("expected order id ord-1, got %s", msg.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for order handler")
	}

	select {
	case msg := <-accountReceived:
		if msg.ID != "acct-1" {
			t.Fatalf("expected account id acct-1, got %s", msg.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for account handler")
	}
}

func TestBridgeMethod(t *testing.T) {
	a := NewMQL5Adapter("127.0.0.1:0")
	if a.Bridge() == nil {
		t.Fatal("expected non-nil bridge from adapter")
	}
}

// waitFor polls a condition until it returns true or the timeout expires.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not met within %v", timeout)
}

// Verify that error sentinels have correct messages.
func TestErrorMessages(t *testing.T) {
	if ErrNotImplemented.Error() != "not implemented" {
		t.Fatalf("unexpected error message: %s", ErrNotImplemented.Error())
	}
	if ErrNotConnected.Error() != "metatrader not connected" {
		t.Fatalf("unexpected error message: %s", ErrNotConnected.Error())
	}
}

func TestMessageConstants(t *testing.T) {
	expected := map[string]string{
		"price":     MsgTypePrice,
		"order":     MsgTypeOrder,
		"position":  MsgTypePosition,
		"account":   MsgTypeAccount,
		"heartbeat": MsgTypeHeartbeat,
		"command":   MsgTypeCommand,
		"response":  MsgTypeResponse,
	}

	for val, constant := range expected {
		if constant != val {
			t.Fatalf("expected %s = %q, got %q", val, val, constant)
		}
	}
}

func TestSendOrderNotConnected(t *testing.T) {
	b := NewBridge("127.0.0.1:0")
	err := b.SendOrder(OrderCommand{Action: "buy", Symbol: "EURUSD", Volume: 0.1})
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterGetPriceNoData(t *testing.T) {
	a := NewMQL5Adapter("127.0.0.1:0")
	_, err := a.GetPrice(nil, "EURUSD")
	if err == nil {
		t.Fatal("expected error for missing price")
	}
	expected := fmt.Sprintf("no price data for %s", "EURUSD")
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}
