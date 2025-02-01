package main

import (
	"net/http"
	"os"

	"github.com/RichardoC/Pad-i/internal/api"
	"github.com/RichardoC/Pad-i/internal/db"
	"github.com/RichardoC/Pad-i/internal/llm"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize database with more detailed error logging
	database, err := db.New("pad-i.db")
	if err != nil {
		logger.Fatal("failed to initialize database",
			zap.Error(err),
			zap.String("dbPath", "pad-i.db"))
	}

	// Initialize LLM service
	llmService, err := llm.New(
		"http://localhost:11434/v1/",
		os.Getenv("OPENAI_API_KEY"),
		"llama3.1:8b",
		database,
	)
	if err != nil {
		logger.Fatal("failed to initialize LLM service", zap.Error(err))
	}

	// Initialize HTTP handler
	handler := api.NewHandler(database, llmService, logger)

	// Set up routes
	http.HandleFunc("/api/message", handler.HandleMessage)
	http.HandleFunc("/api/conversations", handler.GetConversations)
	http.HandleFunc("/api/messages", handler.GetMessages)
	http.HandleFunc("/api/knowledge/search", handler.SearchKnowledge)
	http.HandleFunc("/api/conversations/delete", handler.DeleteConversation)
	http.HandleFunc("/api/conversations/update", handler.UpdateConversation)

	// Serve static files
	fs := http.FileServer(http.Dir("web"))
	http.Handle("/", fs)

	// Start server
	logger.Info("Starting server on :8100")
	if err := http.ListenAndServe(":8100", nil); err != nil {
		logger.Fatal("failed to start server", zap.Error(err))
	}
}
