// Package contracts defines the versioned boundary between compiler, workflows and analytics.
package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

const SignalName = "aisleflow.reroute.v1"
const TaskQueue = "aisleflow-picking"
const MovementSpan = "warehouse.movement"

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func ValidID(s string) bool { return identifier.MatchString(s) }

func WorkflowID(tenant, wave string) string { return "picking/" + tenant + "/" + wave }

type Input struct {
	TenantID string `json:"tenant_id"`
	SiteID   string `json:"site_id"`
	WaveID   string `json:"wave_id"`
	// Variables select explicit exclusive-gateway branches. They are immutable during a run.
	Variables map[string]string `json:"variables,omitempty"`
}

func (i Input) Validate() error {
	if !ValidID(i.TenantID) || !ValidID(i.SiteID) || !ValidID(i.WaveID) {
		return fmt.Errorf("tenant, site and wave must be 1-64 ASCII identifier characters")
	}
	return nil
}

type Step struct {
	ID          string `json:"id"`
	Aisle       string `json:"aisle"`
	Alternative string `json:"alternative"`
}

type Movement struct {
	Input
	TaskID string `json:"task_id"`
	Aisle  string `json:"aisle"`
}

type Reroute struct {
	AlertID    string `json:"alert_id"`
	TenantID   string `json:"tenant_id"`
	SiteID     string `json:"site_id"`
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Aisle      string `json:"aisle"`
}

type Result struct {
	Completed      []string `json:"completed"`
	Aisles         []string `json:"aisles"`
	AcceptedAlerts []string `json:"accepted_alerts"`
}

func StableID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
