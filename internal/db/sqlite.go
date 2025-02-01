package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/RichardoC/Pad-i/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS knowledge (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    conversation_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL
);

-- Drop existing FTS table if it exists
DROP TABLE IF EXISTS knowledge_fts;

-- Recreate FTS table with correct schema
CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts4(
    content,
    conversation_id,
    tokenize=porter
);

-- Trigger to keep the FTS index up to date
CREATE TRIGGER IF NOT EXISTS knowledge_ai AFTER INSERT ON knowledge BEGIN
    INSERT INTO knowledge_fts(docid, content, conversation_id) 
    VALUES (new.id, new.content, new.conversation_id);
END;

CREATE TRIGGER IF NOT EXISTS knowledge_ad AFTER DELETE ON knowledge BEGIN
    DELETE FROM knowledge_fts WHERE docid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS knowledge_au AFTER UPDATE ON knowledge BEGIN
    DELETE FROM knowledge_fts WHERE docid = old.id;
    INSERT INTO knowledge_fts(docid, content, conversation_id) 
    VALUES (new.id, new.content, new.conversation_id);
END;`

type Database struct {
	db *sql.DB
}

func New(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return &Database{db: db}, nil
}

func (db *Database) SaveMessage(msg *models.Message) error {
	query := `
        INSERT INTO messages (conversation_id, role, content, created_at)
        VALUES (?, ?, ?, CURRENT_TIMESTAMP)
        RETURNING id, created_at`

	return db.db.QueryRow(query, msg.ConvID, msg.Role, msg.Content).Scan(&msg.ID, &msg.CreatedAt)
}

func (db *Database) CreateConversation(title string) (*models.Conversation, error) {
	query := `
        INSERT INTO conversations (title, created_at)
        VALUES (?, CURRENT_TIMESTAMP)
        RETURNING id, created_at`

	conv := &models.Conversation{Title: title}
	err := db.db.QueryRow(query, title).Scan(&conv.ID, &conv.CreatedAt)
	return conv, err
}

func (db *Database) SaveToKnowledgeBase(content string, conversationID int64) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert into main knowledge table
	result, err := tx.Exec(`
		INSERT INTO knowledge (content, conversation_id, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, content, conversationID)
	if err != nil {
		return err
	}

	// Get the ID of the inserted row
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	// Insert into FTS table
	_, err = tx.Exec(`
		INSERT INTO knowledge_fts(docid, content, conversation_id)
		VALUES (?, ?, ?)
	`, id, content, conversationID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (db *Database) GetConversationHistory(conversationID int64, limit int) ([]models.Message, error) {
	query := `
        SELECT id, conversation_id, role, content, created_at
        FROM messages
        WHERE conversation_id = ?
        ORDER BY created_at DESC
        LIMIT ?`

	rows, err := db.db.Query(query, conversationID, limit)
	if err != nil {
		return []models.Message{}, err
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var msg models.Message
		err := rows.Scan(&msg.ID, &msg.ConvID, &msg.Role, &msg.Content, &msg.CreatedAt)
		if err != nil {
			return []models.Message{}, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (db *Database) GetConversations() ([]models.Conversation, error) {
	query := `
        SELECT id, title, created_at
        FROM conversations
        ORDER BY created_at DESC`

	rows, err := db.db.Query(query)
	if err != nil {
		return []models.Conversation{}, err
	}
	defer rows.Close()

	conversations := make([]models.Conversation, 0)
	for rows.Next() {
		var conv models.Conversation
		err := rows.Scan(&conv.ID, &conv.Title, &conv.CreatedAt)
		if err != nil {
			return []models.Conversation{}, err
		}
		conversations = append(conversations, conv)
	}
	return conversations, nil
}

func (db *Database) SearchKnowledge(query string) ([]struct {
	Content        string    `json:"content"`
	ConversationID int64     `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
}, error) {
	// Using SQLite FTS4 for full-text search
	const createFTS = `
	CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts4(
		content,
		conversation_id,
		tokenize=porter
	);`
	if _, err := db.db.Exec(createFTS); err != nil {
		return nil, fmt.Errorf("failed to create FTS table: %w", err)
	}

	// Ensure existing knowledge is in FTS table
	const syncFTS = `
	INSERT INTO knowledge_fts(docid, content, conversation_id)
	SELECT id, content, conversation_id 
	FROM knowledge k
	WHERE NOT EXISTS (
		SELECT 1 FROM knowledge_fts f
		WHERE f.docid = k.id
	);`
	if _, err := db.db.Exec(syncFTS); err != nil {
		return nil, fmt.Errorf("failed to sync FTS table: %w", err)
	}

	// Perform the search
	rows, err := db.db.Query(`
		SELECT k.content, k.conversation_id, k.created_at
		FROM knowledge k
		JOIN knowledge_fts fts ON k.id = fts.docid
		WHERE fts.content MATCH ?
		ORDER BY k.created_at DESC;
	`, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search knowledge: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Content        string    `json:"content"`
		ConversationID int64     `json:"conversation_id"`
		CreatedAt      time.Time `json:"created_at"`
	}

	for rows.Next() {
		var result struct {
			Content        string    `json:"content"`
			ConversationID int64     `json:"conversation_id"`
			CreatedAt      time.Time `json:"created_at"`
		}
		if err := rows.Scan(&result.Content, &result.ConversationID, &result.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	return results, nil
}

func (db *Database) DeleteConversation(id int64) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete messages
	if _, err := tx.Exec("DELETE FROM messages WHERE conversation_id = ?", id); err != nil {
		return err
	}

	// Delete knowledge entries
	if _, err := tx.Exec("DELETE FROM knowledge WHERE conversation_id = ?", id); err != nil {
		return err
	}

	// Delete conversation
	if _, err := tx.Exec("DELETE FROM conversations WHERE id = ?", id); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *Database) UpdateConversationTitle(id int64, title string) error {
	_, err := db.db.Exec("UPDATE conversations SET title = ? WHERE id = ?", title, id)
	return err
}
