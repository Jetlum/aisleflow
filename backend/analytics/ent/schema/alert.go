package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Alert is both an audit record and a transactional outbox item.
type Alert struct{ ent.Schema }

func (Alert) Mixin() []ent.Mixin { return []ent.Mixin{IdentityMixin{}} }
func (Alert) Fields() []ent.Field {
	return []ent.Field{
		field.String("alert_id").Immutable(), field.String("site_id").Immutable(), field.String("aisle").Immutable(),
		field.String("workflow_id").Immutable(), field.String("run_id").Immutable(),
		field.Float("mean_seconds").Immutable(), field.Float("threshold_seconds").Immutable(),
		field.Time("delivered_at").Optional().Nillable(), field.String("last_error").Default(""),
		field.Int("attempts").Default(0), field.Time("next_attempt_at").Optional().Nillable(),
		field.Bool("dead").Default(false),
	}
}
func (Alert) Indexes() []ent.Index {
	return []ent.Index{index.Fields("tenant_id", "alert_id").Unique(), index.Fields("tenant_id", "dead", "delivered_at")}
}
