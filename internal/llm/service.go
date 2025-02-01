package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RichardoC/Pad-i/internal/db"
	"github.com/RichardoC/Pad-i/internal/models"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type Service struct {
	llm llms.LLM
	db  *db.Database
}

type StoreInfo struct {
	UserInput   []string `json:"user_input"`   // Array of user messages
	BotResponse []string `json:"bot_response"` // Array of assistant responses
}

type LLMResponse struct {
	Action    string    `json:"action"`     // "reply", "store", "search", or "new_conversation"
	Content   string    `json:"content"`    // The actual response content
	StoreInfo StoreInfo `json:"store_info"` // Optional: Information to store in knowledge base
	NewTitle  string    `json:"new_title"`  // Optional: Title for new conversation
}

type KnowledgeSearchResult struct {
	Content   string    `json:"content"`
	Relevance float64   `json:"relevance"`
	CreatedAt time.Time `json:"created_at"`
}

func New(baseURL, token, model string, database *db.Database) (*Service, error) {
	llm, err := openai.New(
		openai.WithToken(token),
		openai.WithBaseURL(baseURL),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, err
	}
	return &Service{llm: llm, db: database}, nil
}

func (s *Service) SearchKnowledge(ctx context.Context, query string) ([]KnowledgeSearchResult, error) {
	// First, get raw search results from database
	results, err := s.db.SearchKnowledge(query)
	if err != nil {
		return nil, fmt.Errorf("failed to search knowledge base: %w", err)
	}

	// Use LLM to evaluate relevance of each result
	var knowledgeResults []KnowledgeSearchResult
	for _, result := range results {
		prompt := fmt.Sprintf(`
		Query: %s
		
		Potential relevant information: %s
		
		Rate the relevance of this information to the query on a scale of 0.0 to 1.0.
		Respond with only the number.`, query, result.Content)

		completion, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate relevance: %w", err)
		}

		relevance, err := strconv.ParseFloat(strings.TrimSpace(completion), 64)
		if err != nil {
			relevance = 0.0
		}

		if relevance > 0.3 { // Only include somewhat relevant results
			knowledgeResults = append(knowledgeResults, KnowledgeSearchResult{
				Content:   result.Content,
				Relevance: relevance,
				CreatedAt: result.CreatedAt,
			})
		}
	}

	// Sort by relevance
	sort.Slice(knowledgeResults, func(i, j int) bool {
		return knowledgeResults[i].Relevance > knowledgeResults[j].Relevance
	})

	return knowledgeResults, nil
}

func (s *Service) ProcessMessage(ctx context.Context, msg models.Message) (*models.Message, error) {
	// First, search for relevant knowledge
	knowledge, err := s.SearchKnowledge(ctx, msg.Content)
	if err != nil {
		// Log but don't fail if knowledge search fails
		fmt.Printf("Warning: failed to search knowledge: %v\n", err)
		// Continue with empty knowledge
		knowledge = []KnowledgeSearchResult{}
	}

	// Build the prompt with system instructions and conversation history
	systemPrompt := `You are an AI assistant that can:
	1. Reply to users (action: "reply")
	2. Store important information in a knowledge base (action: "store")
	3. Search existing knowledge (action: "search")
	4. Create new conversations when topics change significantly (action: "new_conversation")

	When storing knowledge:
	- Only store specific, important facts or information
	- Extract and summarize the key information, don't store entire conversations
	- Format the information clearly and concisely


	When the user asks about previous information or references past conversations,
	use the "reply" action to respond using the conversation history and knowledge provided.
	Only use "search" when explicitly asked to search for something.

	IMPORTANT: Your response must be a valid JSON object, but the "content" field should contain
	your natural language response to the user, not JSON or technical details.

	Respond with a JSON object containing:
	{
		"action": "reply|store|search|new_conversation",
		"content": "Your natural language response here...",
		"store_info": {
			"user_input": ["The key information to store"],  // Extract only the important facts
			"bot_response": ["Confirmation or clarification of the stored info"]
		},
		"new_title": "optional: title for new conversation if action is new_conversation"
	}`

	// Get conversation history
	history, err := s.db.GetConversationHistory(msg.ConvID, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Build conversation context with knowledge
	prompt := systemPrompt + "\n\nRelevant knowledge from database:\n"
	for _, k := range knowledge {
		prompt += fmt.Sprintf("- %s (relevance: %.2f)\n", k.Content, k.Relevance)
	}

	prompt += "\n\nConversation history:\n"
	for i := len(history) - 1; i >= 0; i-- {
		prompt += fmt.Sprintf("%s: %s\n", history[i].Role, history[i].Content)
	}
	prompt += fmt.Sprintf("\nCurrent message:\n%s: %s\n\nResponse:", msg.Role, msg.Content)

	// Get response from LLM with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	completion, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate completion: %w", err)
	}

	// Parse the JSON response
	var llmResponse LLMResponse
	if err := json.Unmarshal([]byte(completion), &llmResponse); err != nil {
		// If JSON parsing fails, treat the entire completion as the content
		fmt.Printf("Warning: failed to parse LLM response as JSON: %v\nRaw response: %s\n", err, completion)
		llmResponse = LLMResponse{
			Action:  "reply",
			Content: completion,
		}
	} else if strings.HasPrefix(llmResponse.Content, "{") || strings.HasPrefix(llmResponse.Content, "[") {
		// If the content looks like JSON, it might be a raw response
		// Try to extract just the message content
		var rawJSON map[string]interface{}
		if err := json.Unmarshal([]byte(llmResponse.Content), &rawJSON); err == nil {
			if content, ok := rawJSON["content"].(string); ok {
				llmResponse.Content = content
			}
		}
	}

	// Clean up any remaining JSON artifacts in the content
	llmResponse.Content = strings.TrimSpace(llmResponse.Content)
	if strings.HasPrefix(llmResponse.Content, "\"") && strings.HasSuffix(llmResponse.Content, "\"") {
		// Remove surrounding quotes if present
		llmResponse.Content = llmResponse.Content[1 : len(llmResponse.Content)-1]
	}

	// Handle the response based on the action
	switch llmResponse.Action {
	case "reply":
		response := &models.Message{
			ConvID:  msg.ConvID,
			Role:    "assistant",
			Content: llmResponse.Content,
		}
		if err := s.db.SaveMessage(response); err != nil {
			return nil, fmt.Errorf("failed to save message: %w", err)
		}
		return response, nil

	case "store":
		// Extract and format the key information
		var storeContent strings.Builder
		storeContent.WriteString("Knowledge Entry:\n")
		for i := 0; i < len(llmResponse.StoreInfo.UserInput); i++ {
			storeContent.WriteString(fmt.Sprintf("Information: %s\n", llmResponse.StoreInfo.UserInput[i]))
			if i < len(llmResponse.StoreInfo.BotResponse) {
				storeContent.WriteString(fmt.Sprintf("Context: %s\n", llmResponse.StoreInfo.BotResponse[i]))
			}
		}

		content := storeContent.String()
		fmt.Printf("Attempting to store knowledge: ConvID=%d, Content=%q\n", msg.ConvID, content)

		// Always use the current conversation ID
		if err := s.db.SaveToKnowledgeBase(content, msg.ConvID); err != nil {
			fmt.Printf("Warning: failed to store knowledge: %v\n", err)
		} else {
			fmt.Printf("Successfully stored knowledge in conversation %d\n", msg.ConvID)
		}
		fallthrough // Fall through to "reply" case

	case "search":
		// For search action, we still want to reply to the user
		response := &models.Message{
			ConvID:  msg.ConvID,
			Role:    "assistant",
			Content: llmResponse.Content,
		}
		if err := s.db.SaveMessage(response); err != nil {
			return nil, fmt.Errorf("failed to save message: %w", err)
		}
		return response, nil

	case "new_conversation":
		// Create new conversation
		newConv, err := s.db.CreateConversation(llmResponse.NewTitle)
		if err != nil {
			return nil, fmt.Errorf("failed to create new conversation: %w", err)
		}

		// Save the response in the new conversation
		response := &models.Message{
			ConvID:  newConv.ID,
			Role:    "assistant",
			Content: llmResponse.Content,
		}
		if err := s.db.SaveMessage(response); err != nil {
			return nil, fmt.Errorf("failed to save message: %w", err)
		}
		return response, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", llmResponse.Action)
	}
}
