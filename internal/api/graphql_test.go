package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestGraphQL_RegisterQuery(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("test", func(_ map[string]interface{}) (interface{}, error) {
		return "ok", nil
	})
	queries := h.ListQueries()
	if len(queries) != 1 || queries[0] != "test" {
		t.Fatalf("expected [test], got %v", queries)
	}
}

func TestGraphQL_RegisterMutation(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterMutation("doSomething", func(_ map[string]interface{}) (interface{}, error) {
		return "done", nil
	})
	mutations := h.ListMutations()
	if len(mutations) != 1 || mutations[0] != "doSomething" {
		t.Fatalf("expected [doSomething], got %v", mutations)
	}
}

func TestGraphQL_ListQueriesAndMutations(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("b", func(_ map[string]interface{}) (interface{}, error) { return nil, nil })
	h.RegisterQuery("a", func(_ map[string]interface{}) (interface{}, error) { return nil, nil })
	h.RegisterMutation("z", func(_ map[string]interface{}) (interface{}, error) { return nil, nil })
	h.RegisterMutation("y", func(_ map[string]interface{}) (interface{}, error) { return nil, nil })

	queries := h.ListQueries()
	if len(queries) != 2 || queries[0] != "a" || queries[1] != "b" {
		t.Fatalf("expected sorted [a b], got %v", queries)
	}
	mutations := h.ListMutations()
	if len(mutations) != 2 || mutations[0] != "y" || mutations[1] != "z" {
		t.Fatalf("expected sorted [y z], got %v", mutations)
	}
}

func TestGraphQL_ExecuteSimpleQuery(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("health", func(_ map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"status": "ok"}, nil
	})

	resp := h.Execute(GraphQLRequest{Query: "{ health }"})
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", resp.Errors)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", resp.Data)
	}
	health, ok := data["health"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected health map, got %T", data["health"])
	}
	if health["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", health["status"])
	}
}

func TestGraphQL_ExecuteQueryWithArgs(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("price", func(args map[string]interface{}) (interface{}, error) {
		symbol := args["symbol"].(string)
		return map[string]interface{}{"symbol": symbol, "price": 42.0}, nil
	})

	resp := h.Execute(GraphQLRequest{Query: `{ price(symbol: "BTC") }`})
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]interface{})
	price := data["price"].(map[string]interface{})
	if price["symbol"] != "BTC" {
		t.Fatalf("expected symbol BTC, got %v", price["symbol"])
	}
}

func TestGraphQL_ExecuteMultipleFields(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("health", func(_ map[string]interface{}) (interface{}, error) {
		return "ok", nil
	})
	h.RegisterQuery("portfolio", func(_ map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"balance": 100.0}, nil
	})

	resp := h.Execute(GraphQLRequest{Query: "{ health portfolio }"})
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]interface{})
	if _, ok := data["health"]; !ok {
		t.Fatal("expected health in response")
	}
	if _, ok := data["portfolio"]; !ok {
		t.Fatal("expected portfolio in response")
	}
}

func TestGraphQL_ExecuteMutation(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterMutation("placeOrder", func(args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"orderId": "123", "status": "filled"}, nil
	})

	resp := h.Execute(GraphQLRequest{Query: `mutation { placeOrder(symbol: "ETH") }`})
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]interface{})
	order := data["placeOrder"].(map[string]interface{})
	if order["orderId"] != "123" {
		t.Fatalf("expected orderId 123, got %v", order["orderId"])
	}
}

func TestGraphQL_ExecuteUnknownField(t *testing.T) {
	h := NewGraphQLHandler()
	resp := h.Execute(GraphQLRequest{Query: "{ nonexistent }"})
	if len(resp.Errors) == 0 {
		t.Fatal("expected error for unknown field")
	}
	if resp.Errors[0].Message != "unknown field: nonexistent" {
		t.Fatalf("unexpected error message: %s", resp.Errors[0].Message)
	}
}

func TestGraphQL_ServeHTTPPostRequest(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("health", func(_ map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"status": "ok"}, nil
	})

	body, _ := json.Marshal(GraphQLRequest{Query: "{ health }"})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp GraphQLResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", resp.Errors)
	}
	data := resp.Data.(map[string]interface{})
	health := data["health"].(map[string]interface{})
	if health["status"] != "ok" {
		t.Fatalf("expected ok, got %v", health["status"])
	}
}

func TestGraphQL_ServeHTTPRejectsNonPost(t *testing.T) {
	h := NewGraphQLHandler()
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	var resp GraphQLResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Errors) == 0 {
		t.Fatal("expected error in response")
	}
}

func Test_parseQuery_ExtractsFields(t *testing.T) {
	fields := parseQuery("{ health portfolio positions }")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	names := []string{fields[0].Name, fields[1].Name, fields[2].Name}
	expected := []string{"health", "portfolio", "positions"}
	for i, n := range names {
		if n != expected[i] {
			t.Fatalf("field %d: expected %s, got %s", i, expected[i], n)
		}
	}
	for _, f := range fields {
		if f.Type != "query" {
			t.Fatalf("expected type query, got %s", f.Type)
		}
	}
}

func Test_parseQuery_HandlesArguments(t *testing.T) {
	fields := parseQuery(`{ price(symbol: "BTC", limit: 10) }`)
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	f := fields[0]
	if f.Name != "price" {
		t.Fatalf("expected field name price, got %s", f.Name)
	}
	if f.Args["symbol"] != "BTC" {
		t.Fatalf("expected symbol BTC, got %v", f.Args["symbol"])
	}
	if f.Args["limit"] != 10.0 {
		t.Fatalf("expected limit 10, got %v", f.Args["limit"])
	}
}

func Test_parseQuery_MutationType(t *testing.T) {
	fields := parseQuery(`mutation { placeOrder(symbol: "ETH") }`)
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Type != "mutation" {
		t.Fatalf("expected mutation type, got %s", fields[0].Type)
	}
	if fields[0].Name != "placeOrder" {
		t.Fatalf("expected placeOrder, got %s", fields[0].Name)
	}
}

func TestGraphQL_RegisterClawtradeSchema(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterClawtradeSchema()

	queries := h.ListQueries()
	expectedQueries := []string{"health", "portfolio", "positions", "price"}
	if len(queries) != len(expectedQueries) {
		t.Fatalf("expected %d queries, got %d: %v", len(expectedQueries), len(queries), queries)
	}
	for i, q := range expectedQueries {
		if queries[i] != q {
			t.Fatalf("expected query %s at index %d, got %s", q, i, queries[i])
		}
	}

	mutations := h.ListMutations()
	if len(mutations) != 1 || mutations[0] != "placeOrder" {
		t.Fatalf("expected [placeOrder], got %v", mutations)
	}

	// Verify resolvers work
	resp := h.Execute(GraphQLRequest{Query: "{ health }"})
	if len(resp.Errors) > 0 {
		t.Fatalf("health query failed: %v", resp.Errors)
	}

	resp = h.Execute(GraphQLRequest{Query: `{ price(symbol: "BTC") }`})
	if len(resp.Errors) > 0 {
		t.Fatalf("price query failed: %v", resp.Errors)
	}
}

func TestGraphQLConcurrentAccess(t *testing.T) {
	h := NewGraphQLHandler()
	h.RegisterQuery("counter", func(_ map[string]interface{}) (interface{}, error) {
		return 1, nil
	})

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := h.Execute(GraphQLRequest{Query: "{ counter }"})
			if len(resp.Errors) > 0 {
				t.Errorf("unexpected error: %v", resp.Errors)
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.RegisterQuery("dynamic", func(_ map[string]interface{}) (interface{}, error) {
				return n, nil
			})
		}(i)
	}

	// Concurrent list
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.ListQueries()
			_ = h.ListMutations()
		}()
	}

	wg.Wait()
}
