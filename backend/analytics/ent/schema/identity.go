package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// IdentityMixin applies storage identity, tenant attribution and immutable creation time.
// Tenant authorization is enforced by repository predicates and PostgreSQL RLS, not this field.
type IdentityMixin struct{ mixin.Schema }

func (IdentityMixin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).Immutable(),
		field.String("tenant_id").NotEmpty().Immutable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}
