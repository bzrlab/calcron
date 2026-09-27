package server

import "encoding/json"

// frame is the on-the-wire command envelope. Operation-specific validation
// remains with the command that owns its fields.
type frame struct {
	ID             string          `json:"id,omitempty"`
	Op             string          `json:"op"`
	Token          string          `json:"token,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	Key            string          `json:"key,omitempty"`
	Event          string          `json:"event,omitempty"`
	After          string          `json:"after,omitempty"`
	At             string          `json:"at,omitempty"`
	Until          string          `json:"until,omitempty"`
	By             string          `json:"by,omitempty"`
	Cooldown       string          `json:"cooldown,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Chain          json.RawMessage `json:"chain,omitempty"`
	Name           string          `json:"name,omitempty"`
	Namespace      string          `json:"namespace,omitempty"`
	ApplicationID  string          `json:"applicationId,omitempty"`
	Workflow       string          `json:"workflow,omitempty"`
	Correlation    string          `json:"correlationKey,omitempty"`
	Calendar       string          `json:"calendar,omitempty"`
	LocalTime      string          `json:"localTime,omitempty"`
	MissedPolicy   string          `json:"missedPolicy,omitempty"`
	DeliveryID     string          `json:"deliveryId,omitempty"`
	TokenID        string          `json:"tokenId,omitempty"`
	SubjectType    string          `json:"subjectType,omitempty"`
	SubjectID      string          `json:"subjectId,omitempty"`
	Before         int64           `json:"before,omitempty"`
}

type reply struct {
	ID    string `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func fail(err string) reply { return reply{Error: err} }
func ok(data any) reply     { return reply{OK: true, Data: data} }
