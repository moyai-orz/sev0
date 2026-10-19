package tools

import (
	"encoding/json"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/invopop/jsonschema"
)

// Genkit's provider adapter decodes numeric arguments through float64. Discord
// snowflakes exceed its exact range, so never coerce a number back into an ID.
// Accept it at the schema boundary and ask the model to resend the source string.
func defineHistoryTool[In any](g *genkit.Genkit, name, description string, fn func(*ai.ToolContext, In) (*RecentMessagesOutput, error)) ai.Tool {
	reflected := (&jsonschema.Reflector{DoNotReference: true}).Reflect(new(In))
	data, _ := json.Marshal(reflected)
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		panic(err)
	}
	properties := schema["properties"].(map[string]any)
	for _, key := range []string{"channel_id", "message_id", "before_id", "author"} {
		if property, ok := properties[key].(map[string]any); ok {
			property["type"] = []string{"string", "number"}
			property["description"] = descriptionText(property) + " Always copy IDs as quoted strings, never JSON numbers."
		}
	}
	return genkit.DefineTool(g, name, description, func(ctx *ai.ToolContext, raw any) (*RecentMessagesOutput, error) {
		input, ok := raw.(map[string]any)
		if !ok {
			return toolResult(nil, fmt.Errorf("expected object tool input"))
		}
		for _, key := range []string{"channel_id", "message_id", "before_id", "author"} {
			if value, present := input[key]; present {
				if _, ok := value.(string); !ok {
					return &RecentMessagesOutput{Messages: []Message{}, Coverage: "Invalid tool arguments: " + key + " must be a quoted string. Copy the exact original ID from the question or a previous tool result and retry. Numeric IDs may have lost precision; do not convert or guess them. No search was performed."}, nil
				}
			}
		}
		data, err := json.Marshal(input)
		if err != nil {
			return toolResult(nil, err)
		}
		var typed In
		if err := json.Unmarshal(data, &typed); err != nil {
			return toolResult(nil, err)
		}
		return fn(ctx, typed)
	}, ai.WithInputSchema(schema))
}

func descriptionText(property map[string]any) string {
	text, _ := property["description"].(string)
	return text
}
