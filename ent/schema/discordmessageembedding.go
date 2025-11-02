package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/pgvector/pgvector-go"
)

// DiscordMessageEmbedding holds the schema definition for the DiscordMessageEmbedding entity.
type DiscordMessageEmbedding struct {
	ent.Schema
}

// Fields of the DiscordMessageEmbedding.
func (DiscordMessageEmbedding) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Immutable(),
		field.String("message_id").NotEmpty().Immutable(),
		field.String("model").NotEmpty().Immutable(),
		field.Other("embedding", pgvector.Vector{}).
			SchemaType(map[string]string{
				dialect.Postgres: "vector",
			}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Edges of the DiscordMessageEmbedding.
func (DiscordMessageEmbedding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("message", DiscordMessage.Type).
			Ref("embeddings").
			Unique().
			Field("message_id").Required().Immutable(),
	}
}

func (DiscordMessageEmbedding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("message_id", "model").Unique(),
	}
}

func (DiscordMessageEmbedding) Annotations() []schema.Annotation {
	return []schema.Annotation{}
}
