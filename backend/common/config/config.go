package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/common/contracts"
)

func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func Required(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

type BaselineRow struct {
	Tenant, Site, Aisle string
	Mean, StdDev        float64
}

func Analyzer(path string) (core.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return core.Config{}, err
	}
	var rows []BaselineRow
	if err = json.Unmarshal(b, &rows); err != nil {
		return core.Config{}, err
	}
	c := core.Config{Baselines: map[core.Key]core.Baseline{}, Window: 3, Sigma: 2.5, Cooldown: time.Minute, WindowTTL: 10 * time.Minute, MaxKeys: 10000}
	for _, r := range rows {
		k := core.Key{Tenant: r.Tenant, Site: r.Site, Aisle: r.Aisle}
		if _, ok := c.Baselines[k]; ok {
			return c, fmt.Errorf("duplicate baseline")
		}
		c.Baselines[k] = core.Baseline{Mean: r.Mean, StdDev: r.StdDev}
	}
	if len(rows) == 0 {
		return c, fmt.Errorf("at least one baseline required")
	}
	return c, nil
}
func Tokens(raw string) (map[string]string, []string, error) {
	var tokens map[string]string
	if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
		return nil, nil, err
	}
	var tenants []string
	seen := map[string]bool{}
	for token, tenant := range tokens {
		if len(token) < 16 || !contracts.ValidID(tenant) {
			return nil, nil, fmt.Errorf("tokens need >=16 characters and valid tenant IDs")
		}
		if !seen[tenant] {
			tenants = append(tenants, tenant)
			seen[tenant] = true
		}
	}
	if len(tokens) == 0 {
		return nil, nil, fmt.Errorf("empty telemetry token map")
	}
	return tokens, tenants, nil
}
