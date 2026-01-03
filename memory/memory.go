package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"tron"
)

type Store struct {
	db            *sql.DB
	maxMessages   int
	maxAgeMinutes int
	cancel        context.CancelFunc
}

func NewStore(dbPath string, maxMessages, maxAgeMinutes int) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Store{
		db:            db,
		maxMessages:   maxMessages,
		maxAgeMinutes: maxAgeMinutes,
		cancel:        cancel,
	}

	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	go s.cleanupLoop(ctx)

	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
		CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);
		CREATE INDEX IF NOT EXISTS idx_messages_expires_at ON messages(expires_at);
	`)
	return err
}

func (s *Store) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.deleteExpiredMessages(); err != nil {
				log.Printf("[memory] cleanup error: %v", err)
			}
		}
	}
}

func (s *Store) deleteExpiredMessages() error {
	result, err := s.db.Exec("DELETE FROM messages WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP")
	if err != nil {
		return err
	}

	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("[memory] deleted %d expired messages", n)
	}

	return nil
}

func (s *Store) AddMessage(chatID, role, content string, expiresInSeconds int) error {
	var expiresAt sql.NullTime
	if expiresInSeconds > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(time.Duration(expiresInSeconds) * time.Second),
			Valid: true,
		}
	}

	_, err := s.db.Exec(
		"INSERT INTO messages (chat_id, role, content, expires_at) VALUES (?, ?, ?, ?)",
		chatID, role, content, expiresAt,
	)
	if err != nil {
		return err
	}

	return s.pruneOldMessages(chatID)
}

func (s *Store) GetHistory(chatID string) ([]tron.Message, error) {
	cutoff := time.Now().Add(-time.Duration(s.maxAgeMinutes) * time.Minute)

	rows, err := s.db.Query(`
		SELECT role, content
		FROM messages
		WHERE chat_id = ?
		  AND timestamp > ?
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		ORDER BY timestamp ASC
		LIMIT ?
	`, chatID, cutoff, s.maxMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []tron.Message
	for rows.Next() {
		var m tron.Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}

	return messages, rows.Err()
}

func (s *Store) pruneOldMessages(chatID string) error {
	cutoff := time.Now().Add(-time.Duration(s.maxAgeMinutes) * time.Minute)
	_, err := s.db.Exec(
		"DELETE FROM messages WHERE chat_id = ? AND timestamp < ?",
		chatID, cutoff,
	)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		DELETE FROM messages WHERE chat_id = ? AND id NOT IN (
			SELECT id FROM messages WHERE chat_id = ? ORDER BY timestamp DESC LIMIT ?
		)
	`, chatID, chatID, s.maxMessages)
	return err
}

func (s *Store) ClearHistory(chatID string) error {
	_, err := s.db.Exec("DELETE FROM messages WHERE chat_id = ?", chatID)
	return err
}

func (s *Store) Close() error {
	s.cancel()
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}
