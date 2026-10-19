package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		// Nullable for rows captured before channel metadata was recorded.
		field.String("guild_id").Optional(),
		field.String("channel_id").Optional(),
		field.String("reply_to_id").Optional(),
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
		edge.To("embeddings", DiscordMessageEmbedding.Type),
	}
}

func (DiscordMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("guild_id", "channel_id", "timestamp"),
		index.Fields("guild_id", "channel_id", "author_id", "timestamp"),
		index.Fields("timestamp").Annotations(entsql.DescColumns("timestamp")),
	}
}
