package testutils

import "testing"

type Scenario struct {
	Name string
	Run  func(*testing.T)
}

func Run(t *testing.T, scenarios []Scenario) {
	t.Helper()
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.Name, scenario.Run)
	}
}
