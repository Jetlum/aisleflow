package temporal

import (
	"context"
	"example.com/aisleflow/backend/common/contracts"
	"go.temporal.io/sdk/client"
)

type Signaler struct{ Client client.Client }

func (s Signaler) Signal(ctx context.Context, r contracts.Reroute) error {
	return s.Client.SignalWorkflow(ctx, r.WorkflowID, r.RunID, contracts.SignalName, r)
}
