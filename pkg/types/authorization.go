package types

type AccessDecisionReason uint8

const (
	DecisionUnspecified AccessDecisionReason = iota
	DecisionAllowed
	DecisionPermissionMissing
	DecisionApplicationInactive
	DecisionApplicationAccessRequired
)
