package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/aisleflow/backend/common/config"
	"example.com/aisleflow/backend/common/contracts"
	"example.com/aisleflow/backend/temporal/generated"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := client.DialContext(ctx, client.Options{HostPort: config.Env("TEMPORAL_ADDRESS", "localhost:7233")})
	if err != nil {
		return err
	}
	defer c.Close()
	i := contracts.Input{TenantID: "demo", SiteID: "hall-1", WaveID: uuid.NewString(), Variables: map[string]string{"service": "express"}}
	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: contracts.WorkflowID(i.TenantID, i.WaveID), TaskQueue: contracts.TaskQueue, WorkflowExecutionTimeout: 2 * time.Minute}, generated.PickingWorkflow, i)
	if err != nil {
		return err
	}
	fmt.Printf("Started %s (run %s)\n", run.GetID(), run.GetRunID())
	var result contracts.Result
	if err = run.Get(ctx, &result); err != nil {
		return err
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	if len(result.AcceptedAlerts) == 0 {
		return fmt.Errorf("workflow finished but no reroute was observed: inspect analyzer/collector logs")
	}
	return nil
}
