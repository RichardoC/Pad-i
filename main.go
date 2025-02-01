package main

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"go.uber.org/zap"
)

func main() {
	// Initialize zap logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx := context.Background()
	llm, err := openai.New(openai.WithToken("fake"), openai.WithBaseURL("http://localhost:11434/v1/"), openai.WithModel("llama3.1:8b"))
	if err != nil {
		logger.Fatal("failed to initialize OpenAI", zap.Error(err))
	}
	prompt := "What would be a good company name for a company that makes colorful socks?"
	completion, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	if err != nil {
		logger.Fatal("failed to generate completion", zap.Error(err))
	}
	fmt.Println(completion)
}
