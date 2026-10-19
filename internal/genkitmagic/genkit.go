// Package genkitmagic takes care of everything related to Genkit
package genkitmagic

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sev0/ent"
	"sev0/internal/genkitmagic/tools"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

const defaultModel = "google/gemma-4-31b-it:free"

type GenkitMagic struct {
	G *genkit.Genkit

	RecentMessagesTool ai.Tool
	SearchHistoryTool  ai.Tool
	MessageContextTool ai.Tool
}

func Init(
	ctx context.Context,
	entClient *ent.Client,
) (GenkitMagic, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		return GenkitMagic{}, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	model := strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	if model == "" {
		model = defaultModel
	}
	router := &compat_oai.OpenAICompatible{
		Provider: "openrouter",
		APIKey:   apiKey,
		BaseURL:  "https://openrouter.ai/api/v1",
	}
	g := genkit.Init(ctx,
		genkit.WithPlugins(router),
		genkit.WithDefaultModel("openrouter/"+model),
	)

	recentMessagesTool := tools.DefineRecentMessagesTool(g, entClient)

	return GenkitMagic{
		G:                  g,
		RecentMessagesTool: recentMessagesTool,
		SearchHistoryTool:  tools.DefineSearchTool(g, entClient),
		MessageContextTool: tools.DefineContextTool(g, entClient),
	}, nil
}
