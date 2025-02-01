package models

import "time"

type Message struct {
    ID        int64     `json:"id"`
    ConvID    int64     `json:"conversation_id"`
    Role      string    `json:"role"` // user, assistant, or system
    Content   string    `json:"content"`
    CreatedAt time.Time `json:"created_at"`
}

type Conversation struct {
    ID        int64     `json:"id"`
    Title     string    `json:"title"`
    CreatedAt time.Time `json:"created_at"`
} 