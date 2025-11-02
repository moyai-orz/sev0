// Package genkitmagic takes care of everything related to Genkit
package genkitmagic

import (
	"context"

	"sev0/ent"
	"sev0/internal/genkitmagic/tools"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type GenkitMagic struct {
	G   *genkit.Genkit
	OAI *openai.OpenAI

	RecentMessagesTool ai.Tool
}

func Init(
	ctx context.Context,
	entClient *ent.Client,
) (GenkitMagic, error) {
	oai := &openai.OpenAI{}
	g := genkit.Init(ctx,
		genkit.WithPlugins(&googlegenai.GoogleAI{}, oai),
		genkit.WithDefaultModel("googleai/gemini-flash-latest"),
	)

	recentMessagesTool := tools.DefineRecentMessagesTool(g, entClient)

	return GenkitMagic{
		G:                  g,
		OAI:                oai,
		RecentMessagesTool: recentMessagesTool,
	}, nil
}
