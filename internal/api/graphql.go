package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// GraphQL types

// GraphQLRequest represents an incoming GraphQL request.
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents the result of a GraphQL execution.
type GraphQLResponse struct {
	Data   interface{}    `json:"data,omitempty"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// GraphQLError represents an error in GraphQL execution.
type GraphQLError struct {
	Message string   `json:"message"`
	Path    []string `json:"path,omitempty"`
}

// FieldResolver resolves a single field.
type FieldResolver func(args map[string]interface{}) (interface{}, error)

// GraphQLHandler handles GraphQL requests with a simple resolver-based approach.
type GraphQLHandler struct {
	mu        sync.RWMutex
	resolvers map[string]FieldResolver // query field name -> resolver
	mutations map[string]FieldResolver // mutation field name -> resolver
}

// NewGraphQLHandler creates a new handler.
func NewGraphQLHandler() *GraphQLHandler {
	return &GraphQLHandler{
		resolvers: make(map[string]FieldResolver),
		mutations: make(map[string]FieldResolver),
	}
}

// RegisterQuery registers a query field resolver.
func (h *GraphQLHandler) RegisterQuery(field string, resolver FieldResolver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resolvers[field] = resolver
}

// RegisterMutation registers a mutation field resolver.
func (h *GraphQLHandler) RegisterMutation(field string, resolver FieldResolver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mutations[field] = resolver
}

// ServeHTTP handles GraphQL HTTP requests (POST with JSON body).
func (h *GraphQLHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(GraphQLResponse{
			Errors: []GraphQLError{{Message: "only POST method is allowed"}},
		})
		return
	}

	var req GraphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GraphQLResponse{
			Errors: []GraphQLError{{Message: "invalid request body: " + err.Error()}},
		})
		return
	}

	resp := h.Execute(req)

	w.Header().Set("Content-Type", "application/json")
	if len(resp.Errors) > 0 && resp.Data == nil {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp)
}

// Execute processes a GraphQL query string.
func (h *GraphQLHandler) Execute(req GraphQLRequest) GraphQLResponse {
	fields := parseQuery(req.Query)
	if len(fields) == 0 {
		return GraphQLResponse{
			Errors: []GraphQLError{{Message: "empty or invalid query"}},
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	data := make(map[string]interface{})
	var errors []GraphQLError

	for _, f := range fields {
		var resolver FieldResolver
		var ok bool

		if f.Type == "mutation" {
			resolver, ok = h.mutations[f.Name]
		} else {
			resolver, ok = h.resolvers[f.Name]
		}

		if !ok {
			errors = append(errors, GraphQLError{
				Message: fmt.Sprintf("unknown field: %s", f.Name),
				Path:    []string{f.Name},
			})
			continue
		}

		result, err := resolver(f.Args)
		if err != nil {
			errors = append(errors, GraphQLError{
				Message: err.Error(),
				Path:    []string{f.Name},
			})
			continue
		}
		data[f.Name] = result
	}

	resp := GraphQLResponse{}
	if len(data) > 0 {
		resp.Data = data
	}
	if len(errors) > 0 {
		resp.Errors = errors
	}
	return resp
}

type parsedField struct {
	Name string
	Args map[string]interface{}
	Type string // "query" or "mutation"
}

// parseQuery extracts field names and arguments from a simple GraphQL query.
// Supports: { field1 field2(arg: "value") }
// Supports: mutation { field1(arg: "value") }
func parseQuery(query string) []parsedField {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	opType := "query"

	// Check for operation keyword
	if strings.HasPrefix(query, "mutation") {
		opType = "mutation"
		query = strings.TrimPrefix(query, "mutation")
		query = strings.TrimSpace(query)
	} else if strings.HasPrefix(query, "query") {
		query = strings.TrimPrefix(query, "query")
		query = strings.TrimSpace(query)
	}

	// Strip optional operation name before the opening brace
	if idx := strings.Index(query, "{"); idx >= 0 {
		query = query[idx:]
	}

	// Remove outer braces
	query = strings.TrimSpace(query)
	if len(query) < 2 || query[0] != '{' {
		return nil
	}
	// Find matching closing brace
	query = query[1:]
	if idx := strings.LastIndex(query, "}"); idx >= 0 {
		query = query[:idx]
	}
	query = strings.TrimSpace(query)

	if query == "" {
		return nil
	}

	var fields []parsedField
	for len(query) > 0 {
		query = strings.TrimSpace(query)
		if query == "" {
			break
		}

		// Extract field name
		nameEnd := strings.IndexAny(query, " \t\n\r({})")
		var name string
		if nameEnd < 0 {
			name = query
			query = ""
		} else {
			name = query[:nameEnd]
			query = query[nameEnd:]
		}

		if name == "" {
			break
		}

		query = strings.TrimSpace(query)
		args := make(map[string]interface{})

		// Check for arguments in parentheses
		if len(query) > 0 && query[0] == '(' {
			end := findMatchingParen(query)
			if end > 0 {
				argsStr := query[1:end]
				args = parseArgs(argsStr)
				query = query[end+1:]
			}
		}

		// Skip sub-selections (fields in braces) for now
		query = strings.TrimSpace(query)
		if len(query) > 0 && query[0] == '{' {
			depth := 0
			for i, ch := range query {
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
					if depth == 0 {
						query = query[i+1:]
						break
					}
				}
			}
		}

		fields = append(fields, parsedField{
			Name: name,
			Args: args,
			Type: opType,
		})
	}

	return fields
}

// findMatchingParen returns the index of the closing paren matching the opening paren at index 0.
func findMatchingParen(s string) int {
	depth := 0
	inString := false
	for i, ch := range s {
		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if inString {
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseArgs parses a simple argument string like: symbol: "BTC", limit: 10
func parseArgs(s string) map[string]interface{} {
	args := make(map[string]interface{})
	s = strings.TrimSpace(s)

	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}

		// Find key
		colonIdx := strings.Index(s, ":")
		if colonIdx < 0 {
			break
		}
		key := strings.TrimSpace(s[:colonIdx])
		s = strings.TrimSpace(s[colonIdx+1:])

		// Parse value
		if len(s) == 0 {
			break
		}

		if s[0] == '"' {
			// String value
			end := 1
			for end < len(s) {
				if s[end] == '"' && s[end-1] != '\\' {
					break
				}
				end++
			}
			if end < len(s) {
				args[key] = s[1:end]
				s = s[end+1:]
			}
		} else {
			// Non-string value (number, bool, etc.)
			end := strings.IndexAny(s, ", )\t\n\r")
			var valStr string
			if end < 0 {
				valStr = s
				s = ""
			} else {
				valStr = s[:end]
				s = s[end:]
			}
			valStr = strings.TrimSpace(valStr)

			// Try to parse as number
			var numVal float64
			if _, err := fmt.Sscanf(valStr, "%f", &numVal); err == nil {
				args[key] = numVal
			} else if valStr == "true" {
				args[key] = true
			} else if valStr == "false" {
				args[key] = false
			} else {
				args[key] = valStr
			}
		}

		// Skip comma
		s = strings.TrimSpace(s)
		if len(s) > 0 && s[0] == ',' {
			s = s[1:]
		}
	}

	return args
}

// RegisterClawtradeSchema registers all built-in Clawtrade resolvers.
func (h *GraphQLHandler) RegisterClawtradeSchema() {
	h.RegisterQuery("health", func(_ map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"status": "ok"}, nil
	})

	h.RegisterQuery("portfolio", func(_ map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"balance": 10000.0, "unrealizedPnl": 0.0, "todayPnl": 0.0,
		}, nil
	})

	h.RegisterQuery("positions", func(_ map[string]interface{}) (interface{}, error) {
		return []interface{}{}, nil
	})

	h.RegisterQuery("price", func(args map[string]interface{}) (interface{}, error) {
		symbol, _ := args["symbol"].(string)
		if symbol == "" {
			return nil, fmt.Errorf("symbol required")
		}
		return map[string]interface{}{"symbol": symbol, "price": 0.0, "source": "paper"}, nil
	})

	h.RegisterMutation("placeOrder", func(args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"orderId": "mock-001", "status": "pending"}, nil
	})
}

// ListQueries returns registered query field names (sorted).
func (h *GraphQLHandler) ListQueries() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	names := make([]string, 0, len(h.resolvers))
	for name := range h.resolvers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListMutations returns registered mutation field names (sorted).
func (h *GraphQLHandler) ListMutations() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	names := make([]string, 0, len(h.mutations))
	for name := range h.mutations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
