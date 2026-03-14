package alert

// Service implements the AlertService interface for agent tools.
type Service struct {
	mgr *Manager
}

// NewService wraps a Manager as an AlertService.
func NewService(mgr *Manager) *Service {
	return &Service{mgr: mgr}
}

// CreateAlert creates a new alert via the manager.
func (s *Service) CreateAlert(alertType, symbol, condition string, threshold float64, expression, message string, oneShot bool) (int64, error) {
	a := &Alert{
		Type:       alertType,
		Symbol:     symbol,
		Condition:  condition,
		Threshold:  threshold,
		Expression: expression,
		Message:    message,
		Enabled:    true,
		OneShot:    oneShot,
	}
	return s.mgr.AddAlert(a)
}

// DeleteAlert removes an alert via the manager.
func (s *Service) DeleteAlert(id int64) error {
	return s.mgr.RemoveAlert(id)
}

// ListAlerts returns all alerts as maps for the agent tool.
func (s *Service) ListAlerts() ([]map[string]any, error) {
	alerts, err := s.mgr.store.ListAll()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for _, a := range alerts {
		m := map[string]any{
			"id":        a.ID,
			"type":      a.Type,
			"symbol":    a.Symbol,
			"condition": a.Condition,
			"threshold": a.Threshold,
			"enabled":   a.Enabled,
			"one_shot":  a.OneShot,
		}
		if a.Expression != "" {
			m["expression"] = a.Expression
		}
		if a.Message != "" {
			m["message"] = a.Message
		}
		if a.LastTriggeredAt != nil {
			m["last_triggered"] = a.LastTriggeredAt.Format("2006-01-02 15:04:05")
		}
		result = append(result, m)
	}
	return result, nil
}
