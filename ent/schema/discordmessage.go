package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DiscordMessage holds the schema definition for the DiscordMessage entity.
type DiscordMessage struct {
	ent.Schema
}

// Fields of the DiscordMessage.
func (DiscordMessage) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Immutable(),
		field.Text("content"),
		field.String("author_id").NotEmpty().Immutable(),
		field.Time("timestamp").Immutable(),
		field.Time("edited_timestamp").Optional(),
	}
}

// Edges of the DiscordMessage.
func (DiscordMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", DiscordUser.Type).
			Ref("messages").
			Unique().
			Field("author_id").Required().Immutable(),
	}
}
