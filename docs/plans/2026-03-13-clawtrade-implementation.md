# Clawtrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an open-source, self-hosted AI Agent platform for trading that connects to multiple exchanges, uses user-provided LLM keys, and learns from trading history.

**Architecture:** Go core engine handles trading, memory, security, and adapters. TypeScript/Bun plugin runtime handles AI agent orchestration, skills, and LLM communication. Communication via JSON-RPC over IPC (stdin/stdout). React web dashboard. CLI in TypeScript.

**Tech Stack:** Go 1.22+, TypeScript 5+, Bun, React 18+, TailwindCSS, SQLite (via go-sqlite3), TradingView Lightweight Charts

**Design Doc:** `docs/plans/2026-03-13-clawtrade-design.md`

---

## Phase Overview

| Phase | Focus | Estimated Tasks |
|-------|-------|-----------------|
| **Phase 1: Foundation MVP** | Core engine, 1 adapter, basic memory, CLI chat, basic security | Tasks 1-20 |
| **Phase 2: Trading Brain** | Full memory engine, risk engine, multi-adapter, web dashboard | Tasks 21-40 |
| **Phase 3: Skills Ecosystem** | Plugin runtime, skill SDK, MCP, multi-agent, event system | Tasks 41-55 |
| **Phase 4: Polish & Community** | TUI, voice, Telegram bot, PWA, community registry, visual builder | Tasks 56-70 |

---

## Phase 1: Foundation MVP

The MVP goal: user can start Clawtrade, connect their LLM API key + Binance API key, chat with AI about BTC, and place a paper trade via CLI.

---

### Task 1: Project Scaffolding (Go)

**Files:**
- Create: `cmd/clawtrade/main.go`
- Create: `internal/version.go`
- Create: `go.mod`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `README.md`

**Step 1: Initialize Go module**

```bash
cd d:/Clawtrade
go mod init github.com/clawtrade/clawtrade
```

**Step 2: Create main entry point**

```go
// cmd/clawtrade/main.go
package main

import (
	"fmt"
	"os"

	"github.com/clawtrade/clawtrade/internal"
)

func main() {
	fmt.Printf("Clawtrade %s\n", internal.Version)
	if len(os.Args) < 2 {
		fmt.Println("Usage: clawtrade <command>")
		fmt.Println("Commands: version, serve")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("Clawtrade %s\n", internal.Version)
	case "serve":
		fmt.Println("Starting Clawtrade server...")
		// Will be implemented in later tasks
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
```

```go
// internal/version.go
package internal

const Version = "0.1.0-dev"
```

**Step 3: Create Makefile**

```makefile
# Makefile
.PHONY: build run test clean

BINARY=clawtrade
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/clawtrade

run: build
	./$(BUILD_DIR)/$(BINARY) serve

test:
	go test ./... -v -race

clean:
	rm -rf $(BUILD_DIR)

lint:
	golangci-lint run ./...
```

**Step 4: Create .gitignore**

```
# .gitignore
bin/
data/
*.exe
*.db
*.enc
*.chain
node_modules/
dist/
.env
vault.enc
```

**Step 5: Build and verify**

Run: `make build && ./bin/clawtrade version`
Expected: `Clawtrade 0.1.0-dev`

**Step 6: Initialize git and commit**

```bash
git init
git add -A
git commit -m "feat: project scaffolding with Go entry point and Makefile"
```

---

### Task 2: Configuration System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config/default.yaml`

**Step 1: Install dependency**

```bash
go get gopkg.in/yaml.v3
```

**Step 2: Write failing test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_DefaultValues(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Path != "data/clawtrade.db" {
		t.Errorf("expected db path data/clawtrade.db, got %s", cfg.Database.Path)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte("server:\n  port: 8080\n")
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	// Host should still be default
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected default host, got %s", cfg.Server.Host)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL (package does not exist)

**Step 4: Write implementation**

```go
// internal/config/config.go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Vault    VaultConfig    `yaml:"vault"`
	Risk     RiskConfig     `yaml:"risk"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type VaultConfig struct {
	Path string `yaml:"path"`
}

type RiskConfig struct {
	MaxRiskPerTrade    float64 `yaml:"max_risk_per_trade"`
	MaxDailyLoss       float64 `yaml:"max_daily_loss"`
	MaxPositions       int     `yaml:"max_positions"`
	MaxLeverage        float64 `yaml:"max_leverage"`
	DefaultMode        string  `yaml:"default_mode"` // "paper" or "live"
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 9090,
		},
		Database: DatabaseConfig{
			Path: "data/clawtrade.db",
		},
		Vault: VaultConfig{
			Path: "data/vault.enc",
		},
		Risk: RiskConfig{
			MaxRiskPerTrade: 0.02,
			MaxDailyLoss:    0.05,
			MaxPositions:    5,
			MaxLeverage:     10,
			DefaultMode:     "paper",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
```

**Step 5: Create default config**

```yaml
# config/default.yaml
server:
  host: "127.0.0.1"
  port: 9090

database:
  path: "data/clawtrade.db"

vault:
  path: "data/vault.enc"

risk:
  max_risk_per_trade: 0.02   # 2%
  max_daily_loss: 0.05       # 5%
  max_positions: 5
  max_leverage: 10
  default_mode: "paper"      # paper or live
```

**Step 6: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/config/ config/ go.mod go.sum
git commit -m "feat: configuration system with YAML support and defaults"
```

---

### Task 3: SQLite Database Layer

**Files:**
- Create: `internal/database/db.go`
- Create: `internal/database/db_test.go`
- Create: `internal/database/migrations.go`

**Step 1: Install dependency**

```bash
go get github.com/mattn/go-sqlite3
```

Note: go-sqlite3 requires CGO. On Windows, ensure GCC is available (via MSYS2/MinGW or use `modernc.org/sqlite` as pure-Go alternative if CGO is not available).

If CGO is not available, use:
```bash
go get modernc.org/sqlite
```

**Step 2: Write failing test**

```go
// internal/database/db_test.go
package database

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Verify tables exist after migration
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='trade_episodes'").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected trade_episodes table, got count %d", count)
	}
}

func TestOpen_MigrationsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db1.Close()

	// Opening again should not fail (migrations already applied)
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	db2.Close()
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/database/ -v`
Expected: FAIL

**Step 4: Write implementation**

```go
// internal/database/db.go
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}
```

```go
// internal/database/migrations.go
package database

import "database/sql"

func migrate(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS trade_episodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol TEXT NOT NULL,
			side TEXT NOT NULL,
			entry_price REAL,
			exit_price REAL,
			size REAL,
			pnl REAL,
			exchange TEXT,
			strategy TEXT,
			reasoning TEXT,
			outcome TEXT,
			emotion_tag TEXT,
			confidence REAL,
			post_mortem TEXT,
			opened_at DATETIME,
			closed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS semantic_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			category TEXT,
			confidence REAL DEFAULT 0.5,
			evidence_count INTEGER DEFAULT 0,
			effectiveness REAL DEFAULT 0.5,
			source TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_profile (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			summary TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			details TEXT,
			reasoning TEXT,
			risk_check TEXT,
			permission TEXT,
			prev_hash TEXT,
			hash TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_graph (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_from TEXT NOT NULL,
			relation TEXT NOT NULL,
			entity_to TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			evidence TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_episodes_symbol ON trade_episodes(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_episodes_opened ON trade_episodes(opened_at)`,
		`CREATE INDEX IF NOT EXISTS idx_rules_category ON semantic_rules(category)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_from ON knowledge_graph(entity_from)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_to ON knowledge_graph(entity_to)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}

	return nil
}
```

**Step 5: Run tests**

Run: `go test ./internal/database/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/database/ go.mod go.sum
git commit -m "feat: SQLite database layer with schema migrations"
```

---

### Task 4: Encrypted Vault

**Files:**
- Create: `internal/security/vault.go`
- Create: `internal/security/vault_test.go`

**Step 1: Write failing test**

```go
// internal/security/vault_test.go
package security

import (
	"path/filepath"
	"testing"
)

func TestVault_StoreAndRetrieve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, err := NewVault(path, "master-password-123")
	if err != nil {
		t.Fatal(err)
	}

	// Store a key
	err = v.Set("binance", "api_key", "my-binance-key")
	if err != nil {
		t.Fatal(err)
	}

	err = v.Set("binance", "secret", "my-binance-secret")
	if err != nil {
		t.Fatal(err)
	}

	// Retrieve
	val, err := v.Get("binance", "api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "my-binance-key" {
		t.Errorf("expected my-binance-key, got %s", val)
	}

	// Save and reload
	err = v.Save()
	if err != nil {
		t.Fatal(err)
	}

	v2, err := OpenVault(path, "master-password-123")
	if err != nil {
		t.Fatal(err)
	}

	val2, err := v2.Get("binance", "api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val2 != "my-binance-key" {
		t.Errorf("expected my-binance-key after reload, got %s", val2)
	}
}

func TestVault_WrongPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, _ := NewVault(path, "correct-password")
	v.Set("test", "key", "value")
	v.Save()

	_, err := OpenVault(path, "wrong-password")
	if err == nil {
		t.Error("expected error with wrong password")
	}
}

func TestVault_ListNamespaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, _ := NewVault(path, "pass")
	v.Set("binance", "key", "val")
	v.Set("openai", "key", "val")

	ns := v.ListNamespaces()
	if len(ns) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(ns))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/security/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/security/vault.go
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iterations = 100000
	saltSize         = 32
	keySize          = 32 // AES-256
)

type Vault struct {
	mu       sync.RWMutex
	path     string
	key      []byte
	salt     []byte
	data     map[string]map[string]string // namespace -> key -> value
}

type vaultFile struct {
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func deriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, keySize, sha256.New)
}

func NewVault(path, password string) (*Vault, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	return &Vault{
		path: path,
		key:  deriveKey(password, salt),
		salt: salt,
		data: make(map[string]map[string]string),
	}, nil
}

func OpenVault(path, password string) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vault file: %w", err)
	}

	var vf vaultFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		return nil, fmt.Errorf("parse vault file: %w", err)
	}

	key := deriveKey(password, vf.Salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := aesGCM.Open(nil, vf.Nonce, vf.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt vault (wrong password?): %w", err)
	}

	var data map[string]map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("parse vault data: %w", err)
	}

	return &Vault{
		path: path,
		key:  key,
		salt: vf.Salt,
		data: data,
	}, nil
}

func (v *Vault) Set(namespace, key, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.data[namespace] == nil {
		v.data[namespace] = make(map[string]string)
	}
	v.data[namespace][key] = value
	return nil
}

func (v *Vault) Get(namespace, key string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ns, ok := v.data[namespace]
	if !ok {
		return "", fmt.Errorf("namespace %q not found", namespace)
	}
	val, ok := ns[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}
	return val, nil
}

func (v *Vault) Delete(namespace, key string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if ns, ok := v.data[namespace]; ok {
		delete(ns, key)
		if len(ns) == 0 {
			delete(v.data, namespace)
		}
	}
}

func (v *Vault) ListNamespaces() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ns := make([]string, 0, len(v.data))
	for k := range v.data {
		ns = append(ns, k)
	}
	sort.Strings(ns)
	return ns
}

func (v *Vault) Save() error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	plaintext, err := json.Marshal(v.data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	vf := vaultFile{
		Salt:       v.salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}

	raw, err := json.Marshal(vf)
	if err != nil {
		return fmt.Errorf("marshal vault file: %w", err)
	}

	return os.WriteFile(v.path, raw, 0600)
}
```

**Step 4: Install x/crypto dependency and run tests**

```bash
go get golang.org/x/crypto
go test ./internal/security/ -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/security/ go.mod go.sum
git commit -m "feat: encrypted vault with AES-256-GCM and PBKDF2"
```

---

### Task 5: Event Bus

**Files:**
- Create: `internal/engine/eventbus.go`
- Create: `internal/engine/eventbus_test.go`

**Step 1: Write failing test**

```go
// internal/engine/eventbus_test.go
package engine

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	var received []Event
	var mu sync.Mutex

	bus.Subscribe("price.update", func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{
		Type: "price.update",
		Data: map[string]any{"symbol": "BTC/USDT", "price": 67180.0},
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Data["symbol"] != "BTC/USDT" {
		t.Errorf("unexpected symbol: %v", received[0].Data["symbol"])
	}
}

func TestEventBus_WildcardSubscribe(t *testing.T) {
	bus := NewEventBus()

	var count int
	var mu sync.Mutex

	bus.Subscribe("price.*", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: "price.update"})
	bus.Publish(Event{Type: "price.alert"})
	bus.Publish(Event{Type: "trade.filled"}) // should not match

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()

	var count int
	var mu sync.Mutex

	id := bus.Subscribe("test", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	bus.Unsubscribe(id)

	bus.Publish(Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 event after unsub, got %d", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/engine/eventbus.go
package engine

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Type      string         `json:"type"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type EventHandler func(Event)

type subscription struct {
	id      uint64
	pattern string
	handler EventHandler
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers []subscription
	nextID      atomic.Uint64
}

func NewEventBus() *EventBus {
	return &EventBus{}
}

func (b *EventBus) Subscribe(pattern string, handler EventHandler) uint64 {
	id := b.nextID.Add(1)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, subscription{
		id:      id,
		pattern: pattern,
		handler: handler,
	})
	b.mu.Unlock()
	return id
}

func (b *EventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subscribers {
		if sub.id == id {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			return
		}
	}
}

func (b *EventBus) Publish(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	b.mu.RLock()
	// Copy matching handlers to avoid holding lock during dispatch
	var handlers []EventHandler
	for _, sub := range b.subscribers {
		if matchPattern(sub.pattern, e.Type) {
			handlers = append(handlers, sub.handler)
		}
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		go h(e)
	}
}

func matchPattern(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == eventType {
		return true
	}
	// Wildcard: "price.*" matches "price.update", "price.alert"
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(eventType, prefix+".")
	}
	return false
}
```

**Step 4: Run tests**

Run: `go test ./internal/engine/ -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/engine/
git commit -m "feat: event bus with pub/sub and wildcard pattern matching"
```

---

### Task 6: Trading Types & Interfaces

**Files:**
- Create: `internal/adapter/types.go`
- Create: `internal/adapter/adapter.go`

**Step 1: Define core trading types**

```go
// internal/adapter/types.go
package adapter

import "time"

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type OrderType string

const (
	OrderTypeMarket OrderType = "MARKET"
	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeStop   OrderType = "STOP"
)

type OrderStatus string

const (
	OrderStatusPending  OrderStatus = "PENDING"
	OrderStatusFilled   OrderStatus = "FILLED"
	OrderStatusCanceled OrderStatus = "CANCELED"
	OrderStatusFailed   OrderStatus = "FAILED"
)

type Price struct {
	Symbol    string    `json:"symbol"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Last      float64   `json:"last"`
	Volume24h float64   `json:"volume_24h"`
	Timestamp time.Time `json:"timestamp"`
}

type Candle struct {
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

type OrderBookEntry struct {
	Price  float64 `json:"price"`
	Amount float64 `json:"amount"`
}

type OrderBook struct {
	Symbol string           `json:"symbol"`
	Bids   []OrderBookEntry `json:"bids"`
	Asks   []OrderBookEntry `json:"asks"`
}

type Order struct {
	ID        string      `json:"id"`
	Symbol    string      `json:"symbol"`
	Side      Side        `json:"side"`
	Type      OrderType   `json:"type"`
	Price     float64     `json:"price,omitempty"`
	Size      float64     `json:"size"`
	Status    OrderStatus `json:"status"`
	Exchange  string      `json:"exchange"`
	FilledAt  float64     `json:"filled_at,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

type Position struct {
	Symbol    string    `json:"symbol"`
	Side      Side      `json:"side"`
	Size      float64   `json:"size"`
	EntryPrice float64  `json:"entry_price"`
	CurrentPrice float64 `json:"current_price"`
	PnL       float64   `json:"pnl"`
	Exchange  string    `json:"exchange"`
	OpenedAt  time.Time `json:"opened_at"`
}

type Balance struct {
	Asset     string  `json:"asset"`
	Free      float64 `json:"free"`
	Locked    float64 `json:"locked"`
	Total     float64 `json:"total"`
}

type AdapterCaps struct {
	Name       string      `json:"name"`
	WebSocket  bool        `json:"websocket"`
	Margin     bool        `json:"margin"`
	Futures    bool        `json:"futures"`
	OrderTypes []OrderType `json:"order_types"`
}
```

**Step 2: Define adapter interface**

```go
// internal/adapter/adapter.go
package adapter

import "context"

// TradingAdapter is the unified interface for all exchange adapters
type TradingAdapter interface {
	// Info
	Name() string
	Capabilities() AdapterCaps

	// Market Data
	GetPrice(ctx context.Context, symbol string) (*Price, error)
	GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]Candle, error)
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)

	// Trading
	PlaceOrder(ctx context.Context, order Order) (*Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOpenOrders(ctx context.Context) ([]Order, error)

	// Account
	GetBalances(ctx context.Context) ([]Balance, error)
	GetPositions(ctx context.Context) ([]Position, error)

	// Lifecycle
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool
}

// DataAdapter provides market data without trading capability
type DataAdapter interface {
	Name() string
	GetData(ctx context.Context, query string) (any, error)
}

// SignalAdapter provides trading signals
type SignalAdapter interface {
	Name() string
	OnSignal(ctx context.Context, handler func(signal any)) error
}
```

**Step 3: Commit**

```bash
git add internal/adapter/
git commit -m "feat: trading types and adapter interfaces"
```

---

### Task 7: Simulation Adapter (Paper Trading)

**Files:**
- Create: `internal/adapter/simulation/simulation.go`
- Create: `internal/adapter/simulation/simulation_test.go`

**Step 1: Write failing test**

```go
// internal/adapter/simulation/simulation_test.go
package simulation

import (
	"context"
	"testing"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

func TestSimAdapter_PlaceMarketOrder(t *testing.T) {
	sim := New("test-sim", 10000) // $10,000 starting balance
	ctx := context.Background()

	if err := sim.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	// Set a price
	sim.SetPrice("BTC/USDT", 67000)

	order, err := sim.PlaceOrder(ctx, adapter.Order{
		Symbol: "BTC/USDT",
		Side:   adapter.SideBuy,
		Type:   adapter.OrderTypeMarket,
		Size:   0.1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != adapter.OrderStatusFilled {
		t.Errorf("expected filled, got %s", order.Status)
	}
	if order.FilledAt != 67000 {
		t.Errorf("expected fill at 67000, got %f", order.FilledAt)
	}

	// Check position
	positions, _ := sim.GetPositions(ctx)
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].Size != 0.1 {
		t.Errorf("expected size 0.1, got %f", positions[0].Size)
	}

	// Check balance reduced
	balances, _ := sim.GetBalances(ctx)
	for _, b := range balances {
		if b.Asset == "USDT" {
			expected := 10000 - (67000 * 0.1)
			if b.Free != expected {
				t.Errorf("expected balance %f, got %f", expected, b.Free)
			}
		}
	}
}

func TestSimAdapter_Capabilities(t *testing.T) {
	sim := New("test", 10000)
	caps := sim.Capabilities()
	if caps.Name != "test" {
		t.Errorf("expected name test, got %s", caps.Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/simulation/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/adapter/simulation/simulation.go
package simulation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

type SimAdapter struct {
	mu             sync.RWMutex
	name           string
	connected      bool
	initialBalance float64
	balances       map[string]float64
	positions      []adapter.Position
	orders         []adapter.Order
	prices         map[string]float64
	nextOrderID    int
}

func New(name string, initialUSDT float64) *SimAdapter {
	return &SimAdapter{
		name:           name,
		initialBalance: initialUSDT,
		balances:       map[string]float64{"USDT": initialUSDT},
		prices:         make(map[string]float64),
	}
}

func (s *SimAdapter) Name() string { return s.name }

func (s *SimAdapter) Capabilities() adapter.AdapterCaps {
	return adapter.AdapterCaps{
		Name:       s.name,
		WebSocket:  false,
		Margin:     false,
		Futures:    false,
		OrderTypes: []adapter.OrderType{adapter.OrderTypeMarket, adapter.OrderTypeLimit},
	}
}

func (s *SimAdapter) SetPrice(symbol string, price float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices[symbol] = price
}

func (s *SimAdapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.prices[symbol]
	if !ok {
		return nil, fmt.Errorf("no price for %s", symbol)
	}

	return &adapter.Price{
		Symbol:    symbol,
		Bid:       p,
		Ask:       p,
		Last:      p,
		Timestamp: time.Now(),
	}, nil
}

func (s *SimAdapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	return nil, fmt.Errorf("candles not available in simulation mode")
}

func (s *SimAdapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.prices[symbol]
	if !ok {
		return nil, fmt.Errorf("no price for %s", symbol)
	}

	return &adapter.OrderBook{
		Symbol: symbol,
		Bids:   []adapter.OrderBookEntry{{Price: p - 1, Amount: 10}},
		Asks:   []adapter.OrderBookEntry{{Price: p + 1, Amount: 10}},
	}, nil
}

func (s *SimAdapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	price, ok := s.prices[order.Symbol]
	if !ok {
		return nil, fmt.Errorf("no price set for %s", order.Symbol)
	}

	cost := price * order.Size

	if order.Side == adapter.SideBuy {
		if s.balances["USDT"] < cost {
			return nil, fmt.Errorf("insufficient balance: need %f, have %f", cost, s.balances["USDT"])
		}
		s.balances["USDT"] -= cost
		s.addPosition(order.Symbol, adapter.SideBuy, order.Size, price)
	} else {
		s.closePosition(order.Symbol, order.Size, price)
		s.balances["USDT"] += cost
	}

	s.nextOrderID++
	filled := order
	filled.ID = fmt.Sprintf("sim-%d", s.nextOrderID)
	filled.Status = adapter.OrderStatusFilled
	filled.FilledAt = price
	filled.Exchange = s.name
	filled.CreatedAt = time.Now()
	s.orders = append(s.orders, filled)

	return &filled, nil
}

func (s *SimAdapter) addPosition(symbol string, side adapter.Side, size, price float64) {
	for i, p := range s.positions {
		if p.Symbol == symbol && p.Side == side {
			// Average into existing position
			totalCost := p.EntryPrice*p.Size + price*size
			totalSize := p.Size + size
			s.positions[i].EntryPrice = totalCost / totalSize
			s.positions[i].Size = totalSize
			return
		}
	}

	s.positions = append(s.positions, adapter.Position{
		Symbol:       symbol,
		Side:         side,
		Size:         size,
		EntryPrice:   price,
		CurrentPrice: price,
		PnL:          0,
		Exchange:     s.name,
		OpenedAt:     time.Now(),
	})
}

func (s *SimAdapter) closePosition(symbol string, size, price float64) {
	for i, p := range s.positions {
		if p.Symbol == symbol {
			s.positions[i].Size -= size
			if s.positions[i].Size <= 0 {
				s.positions = append(s.positions[:i], s.positions[i+1:]...)
			}
			return
		}
	}
}

func (s *SimAdapter) CancelOrder(ctx context.Context, orderID string) error {
	return fmt.Errorf("market orders cannot be cancelled in simulation")
}

func (s *SimAdapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	return nil, nil
}

func (s *SimAdapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var balances []adapter.Balance
	for asset, amount := range s.balances {
		balances = append(balances, adapter.Balance{
			Asset: asset,
			Free:  amount,
			Total: amount,
		})
	}
	return balances, nil
}

func (s *SimAdapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Update current prices and PnL
	result := make([]adapter.Position, len(s.positions))
	for i, p := range s.positions {
		result[i] = p
		if price, ok := s.prices[p.Symbol]; ok {
			result[i].CurrentPrice = price
			if p.Side == adapter.SideBuy {
				result[i].PnL = (price - p.EntryPrice) * p.Size
			} else {
				result[i].PnL = (p.EntryPrice - price) * p.Size
			}
		}
	}
	return result, nil
}

func (s *SimAdapter) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = true
	return nil
}

func (s *SimAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	return nil
}

func (s *SimAdapter) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}
```

**Step 4: Run tests**

Run: `go test ./internal/adapter/simulation/ -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/adapter/simulation/
git commit -m "feat: simulation adapter for paper trading"
```

---

### Task 8: Audit Log with Hash Chain

**Files:**
- Create: `internal/security/audit.go`
- Create: `internal/security/audit_test.go`

**Step 1: Write failing test**

```go
// internal/security/audit_test.go
package security

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAuditLog_WriteAndVerify(t *testing.T) {
	db := setupTestDB(t)
	audit := NewAuditLog(db)

	err := audit.Log("agent:trader", "PLACE_ORDER", map[string]any{
		"symbol": "BTC/USDT",
		"side":   "BUY",
		"size":   0.1,
	}, "RSI oversold")
	if err != nil {
		t.Fatal(err)
	}

	err = audit.Log("agent:trader", "PLACE_ORDER", map[string]any{
		"symbol": "ETH/USDT",
		"side":   "SELL",
		"size":   1.0,
	}, "Taking profit")
	if err != nil {
		t.Fatal(err)
	}

	// Verify chain integrity
	valid, err := audit.VerifyChain()
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Error("chain should be valid")
	}
}

func TestAuditLog_TamperDetection(t *testing.T) {
	db := setupTestDB(t)
	audit := NewAuditLog(db)

	audit.Log("agent", "ACTION1", nil, "")
	audit.Log("agent", "ACTION2", nil, "")

	// Tamper with a record
	db.Exec("UPDATE audit_log SET action='TAMPERED' WHERE id=1")

	valid, err := audit.VerifyChain()
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Error("tampered chain should be invalid")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/security/ -v -run TestAudit`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/security/audit.go
package security

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type AuditLog struct {
	db       *sql.DB
	lastHash string
}

type AuditEntry struct {
	ID        int64          `json:"id"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Details   map[string]any `json:"details,omitempty"`
	Reasoning string         `json:"reasoning,omitempty"`
	PrevHash  string         `json:"prev_hash"`
	Hash      string         `json:"hash"`
	CreatedAt time.Time      `json:"created_at"`
}

func NewAuditLog(db *sql.DB) *AuditLog {
	al := &AuditLog{db: db}

	// Load last hash
	var hash sql.NullString
	db.QueryRow("SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1").Scan(&hash)
	if hash.Valid {
		al.lastHash = hash.String
	}

	return al
}

func (al *AuditLog) Log(actor, action string, details map[string]any, reasoning string) error {
	detailsJSON, _ := json.Marshal(details)
	now := time.Now()

	// Compute hash
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		al.lastHash, actor, action, string(detailsJSON), reasoning, now.Format(time.RFC3339Nano))
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(hashInput)))

	_, err := al.db.Exec(
		`INSERT INTO audit_log (actor, action, details, reasoning, prev_hash, hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		actor, action, string(detailsJSON), reasoning, al.lastHash, hash, now,
	)
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	al.lastHash = hash
	return nil
}

func (al *AuditLog) VerifyChain() (bool, error) {
	rows, err := al.db.Query(
		"SELECT id, actor, action, details, reasoning, prev_hash, hash, created_at FROM audit_log ORDER BY id ASC",
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	prevHash := ""
	for rows.Next() {
		var entry AuditEntry
		var detailsStr string
		var createdAt string
		err := rows.Scan(&entry.ID, &entry.Actor, &entry.Action, &detailsStr,
			&entry.Reasoning, &entry.PrevHash, &entry.Hash, &createdAt)
		if err != nil {
			return false, err
		}

		if entry.PrevHash != prevHash {
			return false, nil
		}

		// Recompute hash
		hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
			entry.PrevHash, entry.Actor, entry.Action, detailsStr, entry.Reasoning, createdAt)
		expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(hashInput)))

		if entry.Hash != expectedHash {
			return false, nil
		}

		prevHash = entry.Hash
	}

	return true, nil
}

func (al *AuditLog) Query(action string, limit int) ([]AuditEntry, error) {
	query := "SELECT id, actor, action, details, reasoning, prev_hash, hash, created_at FROM audit_log"
	var args []any

	if action != "" {
		query += " WHERE action = ?"
		args = append(args, action)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := al.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailsStr, createdAt string
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &detailsStr,
			&e.Reasoning, &e.PrevHash, &e.Hash, &createdAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(detailsStr), &e.Details)
		entries = append(entries, e)
	}
	return entries, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/security/ -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/security/audit.go internal/security/audit_test.go
git commit -m "feat: cryptographic audit log with hash chain and tamper detection"
```

---

### Task 9: Basic Memory Store (Episodic + Semantic)

**Files:**
- Create: `internal/memory/store.go`
- Create: `internal/memory/store_test.go`

**Step 1: Write failing test**

```go
// internal/memory/store_test.go
package memory

import (
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func TestMemoryStore_SaveAndQueryEpisodes(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	err := store.SaveEpisode(Episode{
		Symbol:    "BTC/USDT",
		Side:      "BUY",
		EntryPrice: 67000,
		ExitPrice:  68000,
		Size:      0.1,
		PnL:       100,
		Exchange:  "binance",
		Strategy:  "rsi-scalp",
		Reasoning: "RSI oversold at 28",
		Outcome:   "win",
	})
	if err != nil {
		t.Fatal(err)
	}

	episodes, err := store.QueryEpisodes("BTC/USDT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(episodes))
	}
	if episodes[0].PnL != 100 {
		t.Errorf("expected PnL 100, got %f", episodes[0].PnL)
	}
}

func TestMemoryStore_SemanticRules(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	id, err := store.SaveRule(Rule{
		Content:    "BTC dumps 70% of the time before FOMC",
		Category:   "macro",
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	rules, err := store.QueryRules("macro", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", rules[0].Confidence)
	}
}

func TestMemoryStore_UserProfile(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	store.SetProfile("risk_tolerance", "2%")
	store.SetProfile("preferred_pairs", "BTC,ETH,SOL")

	val, _ := store.GetProfile("risk_tolerance")
	if val != "2%" {
		t.Errorf("expected 2%%, got %s", val)
	}

	all, _ := store.GetAllProfile()
	if len(all) != 2 {
		t.Errorf("expected 2 profile entries, got %d", len(all))
	}
}

func TestMemoryStore_Conversations(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	store.SaveMessage("user", "Should I long BTC?")
	store.SaveMessage("assistant", "Based on RSI analysis...")

	msgs, _ := store.GetRecentMessages(10)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected first message from user, got %s", msgs[0].Role)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/memory/store.go
package memory

import (
	"database/sql"
	"time"
)

type Episode struct {
	ID         int64   `json:"id"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	EntryPrice float64 `json:"entry_price"`
	ExitPrice  float64 `json:"exit_price"`
	Size       float64 `json:"size"`
	PnL        float64 `json:"pnl"`
	Exchange   string  `json:"exchange"`
	Strategy   string  `json:"strategy"`
	Reasoning  string  `json:"reasoning"`
	Outcome    string  `json:"outcome"`
	EmotionTag string  `json:"emotion_tag"`
	Confidence float64 `json:"confidence"`
	PostMortem string  `json:"post_mortem"`
	OpenedAt   time.Time `json:"opened_at"`
	ClosedAt   time.Time `json:"closed_at"`
}

type Rule struct {
	ID            int64   `json:"id"`
	Content       string  `json:"content"`
	Category      string  `json:"category"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidence_count"`
	Effectiveness float64 `json:"effectiveness"`
	Source        string  `json:"source"`
}

type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Episodes

func (s *Store) SaveEpisode(ep Episode) error {
	if ep.OpenedAt.IsZero() {
		ep.OpenedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO trade_episodes (symbol, side, entry_price, exit_price, size, pnl, exchange, strategy, reasoning, outcome, emotion_tag, confidence, post_mortem, opened_at, closed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.Symbol, ep.Side, ep.EntryPrice, ep.ExitPrice, ep.Size, ep.PnL,
		ep.Exchange, ep.Strategy, ep.Reasoning, ep.Outcome, ep.EmotionTag,
		ep.Confidence, ep.PostMortem, ep.OpenedAt, ep.ClosedAt,
	)
	return err
}

func (s *Store) QueryEpisodes(symbol string, limit int) ([]Episode, error) {
	query := "SELECT id, symbol, side, entry_price, exit_price, size, pnl, exchange, strategy, reasoning, outcome, emotion_tag, confidence, post_mortem, opened_at, closed_at FROM trade_episodes"
	var args []any

	if symbol != "" {
		query += " WHERE symbol = ?"
		args = append(args, symbol)
	}
	query += " ORDER BY opened_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var ep Episode
		err := rows.Scan(&ep.ID, &ep.Symbol, &ep.Side, &ep.EntryPrice, &ep.ExitPrice,
			&ep.Size, &ep.PnL, &ep.Exchange, &ep.Strategy, &ep.Reasoning,
			&ep.Outcome, &ep.EmotionTag, &ep.Confidence, &ep.PostMortem,
			&ep.OpenedAt, &ep.ClosedAt)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, ep)
	}
	return episodes, nil
}

// Rules

func (s *Store) SaveRule(rule Rule) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO semantic_rules (content, category, confidence, evidence_count, effectiveness, source)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rule.Content, rule.Category, rule.Confidence, rule.EvidenceCount, rule.Effectiveness, rule.Source,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) QueryRules(category string, limit int) ([]Rule, error) {
	query := "SELECT id, content, category, confidence, evidence_count, effectiveness, source FROM semantic_rules"
	var args []any

	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}
	query += " ORDER BY confidence DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		err := rows.Scan(&r.ID, &r.Content, &r.Category, &r.Confidence,
			&r.EvidenceCount, &r.Effectiveness, &r.Source)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Store) UpdateRuleConfidence(id int64, confidence float64, evidenceCount int) error {
	_, err := s.db.Exec(
		"UPDATE semantic_rules SET confidence = ?, evidence_count = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		confidence, evidenceCount, id,
	)
	return err
}

// User Profile

func (s *Store) SetProfile(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_profile (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

func (s *Store) GetProfile(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM user_profile WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) GetAllProfile() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM user_profile")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profile := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		profile[k] = v
	}
	return profile, nil
}

// Conversations

func (s *Store) SaveMessage(role, content string) error {
	_, err := s.db.Exec(
		"INSERT INTO conversations (role, content) VALUES (?, ?)",
		role, content,
	)
	return err
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, role, content, created_at FROM conversations ORDER BY id ASC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt)
		messages = append(messages, m)
	}
	return messages, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/memory/ -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/memory/
git commit -m "feat: memory store with episodic, semantic, profile, and conversation layers"
```

---

### Task 10: REST API Server

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`
- Create: `internal/api/handlers.go`

**Step 1: Install dependencies**

```bash
go get github.com/go-chi/chi/v5
go get github.com/go-chi/cors
```

**Step 2: Write failing test**

```go
// internal/api/server_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawtrade/clawtrade/internal/engine"
)

func TestHealthEndpoint(t *testing.T) {
	bus := engine.NewEventBus()
	srv := NewServer(bus, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/system/health", nil)
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/api/ -v`
Expected: FAIL

**Step 4: Write implementation**

```go
// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/engine"
	"github.com/clawtrade/clawtrade/internal/memory"
	"github.com/clawtrade/clawtrade/internal/security"
)

type Server struct {
	router   chi.Router
	bus      *engine.EventBus
	memory   *memory.Store
	audit    *security.AuditLog
	adapters map[string]adapter.TradingAdapter
}

func NewServer(bus *engine.EventBus, mem *memory.Store, audit *security.AuditLog, adapters map[string]adapter.TradingAdapter) *Server {
	s := &Server{
		bus:      bus,
		memory:   mem,
		audit:    audit,
		adapters: adapters,
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/system/health", s.handleHealth)
		r.Get("/system/version", s.handleVersion)
	})

	s.router = r
	return s
}

func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("API server listening on %s\n", addr)
	return srv.ListenAndServe()
}
```

```go
// internal/api/handlers.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/clawtrade/clawtrade/internal"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":  "ok",
		"version": internal.Version,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"version": internal.Version,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

**Step 5: Run tests**

Run: `go test ./internal/api/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/ go.mod go.sum
git commit -m "feat: REST API server with chi router and health endpoint"
```

---

### Task 11-20: Remaining Phase 1 Tasks (Summary)

The following tasks complete Phase 1 MVP. Each follows the same TDD pattern.

#### Task 11: IPC Bridge (Go ↔ TypeScript)
- `internal/plugin/ipc.go` - JSON-RPC server over stdin/stdout
- Go core spawns Bun subprocess, communicates via JSON-RPC
- Methods: `llm.chat`, `llm.analyze`, `memory.query`, `market.price`

#### Task 12: Plugin Runtime Scaffolding (TypeScript/Bun)
- `plugins/runtime/package.json`, `tsconfig.json`
- `plugins/runtime/src/index.ts` - IPC client, receives JSON-RPC calls from Go
- `plugins/runtime/src/ipc.ts` - JSON-RPC protocol handler

#### Task 13: LLM Adapter (TypeScript)
- `plugins/runtime/src/llm/adapter.ts` - unified LLM interface
- `plugins/runtime/src/llm/claude.ts` - Anthropic SDK integration
- `plugins/runtime/src/llm/openai.ts` - OpenAI SDK integration
- `plugins/runtime/src/llm/router.ts` - model selection + fallback

#### Task 14: AI Agent (Basic Chat)
- `plugins/runtime/src/agent/chat.ts` - basic chat agent
- System prompt with trading context
- Memory injection into prompt
- Conversation history management

#### Task 15: Memory Retriever
- `internal/memory/retriever.go` - query relevant memories for a given context
- Keyword matching (v1, vector search in Phase 2)
- Multi-layer scan: episodes + rules + profile + conversations
- Budget packing: fit results into token limit

#### Task 16: CLI Scaffolding (TypeScript)
- `cli/package.json`, `tsconfig.json`
- `cli/src/index.ts` - entry point, arg parsing
- `cli/src/repl.ts` - interactive chat mode
- Connects to Go core API via HTTP

#### Task 17: CLI Chat Integration
- Wire CLI REPL → API → Plugin Runtime → LLM → Response
- Display streaming AI responses
- Show memory context indicators
- Commands: `/portfolio`, `/price BTC`, `/quit`

#### Task 18: Wire Everything Together (main.go)
- Update `cmd/clawtrade/main.go` to:
  - Load config
  - Open database
  - Initialize vault
  - Create event bus
  - Start plugin runtime (spawn Bun process)
  - Start API server
  - Graceful shutdown

#### Task 19: Setup & First-Run Experience
- `clawtrade init` - guided setup wizard
  - Set master password for vault
  - Add LLM API key (Claude/OpenAI)
  - Add exchange API key (optional, can use paper mode)
  - Choose default mode (paper/live)

#### Task 20: End-to-End Test
- Integration test: start server → connect CLI → chat with AI → place paper trade
- Verify: config loaded, vault encrypted, memory stored, audit logged, paper trade executed

---

## Phase 2: Trading Brain (Tasks 21-40)

| Task | Component | Description |
|------|-----------|-------------|
| 21 | Memory: Vector Search | Embed memories locally, vector similarity retrieval |
| 22 | Memory: Knowledge Graph | Entity/relation store, graph traversal |
| 23 | Memory: Consolidation | Periodic episodic→semantic rule extraction via LLM |
| 24 | Memory: Emotional Tracker | Detect TILT/FOMO/FEAR from behavior patterns |
| 25 | Memory: Temporal Patterns | Time-of-day, day-of-week, seasonal analysis |
| 26 | Memory: Meta-Memory | Track memory effectiveness, auto-promote/demote |
| 27 | Risk: Pre-Trade Engine | Position size, max risk, correlation checks |
| 28 | Risk: What-If Simulation | Monte Carlo, scenario analysis before trade |
| 29 | Risk: Circuit Breakers | Daily loss halt, consecutive loss pause, panic button |
| 30 | Risk: Live Monitor | Position tracking, drawdown alerts, trailing stops |
| 31 | Adapter: Binance | Real Binance REST + WebSocket adapter |
| 32 | Adapter: Bybit | Bybit adapter |
| 33 | Adapter: OKX | OKX adapter |
| 34 | Adapter: Adapter Manager | Registry, health check, failover, rate limiting |
| 35 | Adapter: Symbol Normalizer | Unified symbol format across exchanges |
| 36 | Security: Watchdog | Independent process monitoring AI behavior |
| 37 | Security: Permission Matrix | Per-action, per-exchange, per-symbol permissions |
| 38 | Security: Dead Man's Switch | Heartbeat monitor, server-side SL enforcement |
| 39 | Web: Dashboard MVP | React app with chart, chat, positions, portfolio widgets |
| 40 | Web: WebSocket Integration | Real-time updates for prices, positions, AI chat |

## Phase 3: Skills Ecosystem (Tasks 41-55)

| Task | Component | Description |
|------|-----------|-------------|
| 41 | Skill SDK | `defineSkill()` API, permission model, tool registration |
| 42 | Skill Loader | Discovery, loading, sandboxing, resource limits |
| 43 | Skill: Technical Analysis | Built-in RSI, MACD, Bollinger, Ichimoku |
| 44 | Skill: Screener | Market scanner across symbols |
| 45 | Skill: News Aggregator | News feed with AI sentiment tagging |
| 46 | Event System | Event subscriptions for skills, built-in event types |
| 47 | Skill Pipeline | Chain skills together, YAML pipeline definition |
| 48 | Multi-Agent | Conductor + specialist agents (analyst, trader, risk) |
| 49 | MCP Client | Consume external MCP servers |
| 50 | MCP Server | Expose Clawtrade as MCP server |
| 51 | Strategy Arena | A/B testing multiple strategies |
| 52 | Skill Self-Optimization | Parameter auto-tuning based on results |
| 53 | AI Skill Generator | Natural language → TypeScript skill |
| 54 | Community Registry | Git-based skill publishing, install, search |
| 55 | Adapter: MQL5 Bridge | MetaTrader Expert Advisor + TCP bridge |

## Phase 4: Polish & Community (Tasks 56-70)

| Task | Component | Description |
|------|-----------|-------------|
| 56 | CLI: Rich TUI | Full terminal dashboard with charts and panels |
| 57 | CLI: Voice | Local Whisper STT, hands-free trading |
| 58 | Web: Widget System | Drag-drop modular widgets, save layouts |
| 59 | Web: Command Palette | Ctrl+K universal search + command + AI |
| 60 | Web: Context-Aware UI | UI adapts to market state + emotional state |
| 61 | Web: Replay Mode | Time machine, replay past sessions |
| 62 | Web: Visual Skill Builder | No-code drag-drop skill creation |
| 63 | Mobile: PWA | Progressive web app with push notifications |
| 64 | Telegram Bot | 2-way bot with inline buttons, daily briefing |
| 65 | Adapter: DEX | Uniswap, Jupiter, Hyperliquid |
| 66 | Adapter: Smart Order Router | Cross-exchange routing, aggregated orderbook |
| 67 | Adapter: Data Adapters | CoinGecko, on-chain, sentiment, economic calendar |
| 68 | GraphQL API | Flexible queries alongside REST |
| 69 | Client SDKs | TypeScript, Python, Go SDKs |
| 70 | Accessibility & i18n | Color blind modes, screen reader, multi-language |

---

*Implementation plan created 2026-03-13. Start with Phase 1, Task 1.*
