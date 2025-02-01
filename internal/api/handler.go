package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"

    "github.com/RichardoC/Pad-i/internal/db"
    "github.com/RichardoC/Pad-i/internal/llm"
    "github.com/RichardoC/Pad-i/internal/models"
    "go.uber.org/zap"
)

type Handler struct {
    db      *db.Database
    llm     *llm.Service
    logger  *zap.Logger
}

func NewHandler(database *db.Database, llmService *llm.Service, logger *zap.Logger) *Handler {
    return &Handler{
        db:     database,
        llm:    llmService,
        logger: logger,
    }
}

type MessageRequest struct {
    Content string `json:"content"`
}

type MessageResponse struct {
    Message *models.Message `json:"message"`
    NewConversationID int64 `json:"new_conversation_id,omitempty"`
}

// Add this new type for conversation creation requests
type CreateConversationRequest struct {
    Title string `json:"title"`
}

type UpdateConversationRequest struct {
    Title string `json:"title"`
}

func (h *Handler) HandleMessage(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Get conversation ID from URL path
    convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
    if err != nil {
        http.Error(w, "Invalid conversation ID", http.StatusBadRequest)
        return
    }

    // Parse request body
    var req MessageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    // Create user message
    userMsg := &models.Message{
        ConvID:  convID,
        Role:    "user",
        Content: req.Content,
    }

    // Save user message
    if err := h.db.SaveMessage(userMsg); err != nil {
        h.logger.Error("Failed to save user message", zap.Error(err))
        http.Error(w, fmt.Sprintf("Failed to save message: %v", err), http.StatusInternalServerError)
        return
    }

    // Process message with LLM
    response, err := h.llm.ProcessMessage(r.Context(), *userMsg)
    if err != nil {
        h.logger.Error("Failed to process message", zap.Error(err))
        http.Error(w, fmt.Sprintf("Failed to process message: %v", err), http.StatusInternalServerError)
        return
    }

    // Send response
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(MessageResponse{
        Message: response,
        NewConversationID: response.ConvID,
    }); err != nil {
        h.logger.Error("Failed to encode response", zap.Error(err))
        http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
        return
    }
}

// Update the GetConversations handler to handle both GET and POST
func (h *Handler) GetConversations(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        conversations, err := h.db.GetConversations()
        if err != nil {
            h.logger.Error("Failed to get conversations", 
                zap.Error(err),
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path))
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        h.logger.Debug("Retrieved conversations",
            zap.Int("count", len(conversations)),
            zap.String("method", r.Method),
            zap.String("path", r.URL.Path))

        // Add CORS headers if needed
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Content-Type", "application/json")
        
        // Add error handling for encoding
        if err := json.NewEncoder(w).Encode(conversations); err != nil {
            h.logger.Error("Failed to encode conversations", zap.Error(err))
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

    case http.MethodPost:
        var req CreateConversationRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request body", http.StatusBadRequest)
            return
        }

        conversation, err := h.db.CreateConversation(req.Title)
        if err != nil {
            h.logger.Error("Failed to create conversation", zap.Error(err))
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(conversation)

    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
    if err != nil {
        http.Error(w, "Invalid conversation ID", http.StatusBadRequest)
        return
    }

    messages, err := h.db.GetConversationHistory(convID, 50)
    if err != nil {
        h.logger.Error("Failed to get messages", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(messages)
}

func (h *Handler) SearchKnowledge(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    query := r.URL.Query().Get("q")
    if query == "" {
        http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
        return
    }

    results, err := h.llm.SearchKnowledge(r.Context(), query)
    if err != nil {
        h.logger.Error("Failed to search knowledge", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(results)
}

func (h *Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodDelete {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
    if err != nil {
        http.Error(w, "Invalid conversation ID", http.StatusBadRequest)
        return
    }

    if err := h.db.DeleteConversation(convID); err != nil {
        h.logger.Error("Failed to delete conversation", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func (h *Handler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPut {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    convID, err := strconv.ParseInt(r.URL.Query().Get("conversation_id"), 10, 64)
    if err != nil {
        http.Error(w, "Invalid conversation ID", http.StatusBadRequest)
        return
    }

    var req UpdateConversationRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    if err := h.db.UpdateConversationTitle(convID, req.Title); err != nil {
        h.logger.Error("Failed to update conversation", zap.Error(err))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
} 