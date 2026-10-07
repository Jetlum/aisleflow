package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Observation struct{ ent.Schema }

func (Observation) Mixin() []ent.Mixin { return []ent.Mixin{IdentityMixin{}} }
func (Observation) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_id").Immutable(), field.String("site_id").Immutable(), field.String("aisle").Immutable(),
		field.String("workflow_id").Immutable(), field.String("run_id").Immutable(), field.String("task_id").Immutable(),
		field.Float("seconds").Positive().Immutable(), field.Time("observed_at").Immutable(),
	}
}
func (Observation) Indexes() []ent.Index {
	return []ent.Index{index.Fields("tenant_id", "event_id").Unique(), index.Fields("tenant_id", "site_id", "aisle", "observed_at")}
}
