package reminder

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type Reminder struct {
	ID            int64
	Prompt        string
	ScheduleType  string // "once", "daily", "hourly", "interval", "cron"
	ScheduleValue string // "08:00", "30", "2h", "0 8 * * *"
	Timezone      string
	Recipient     string // channel to send reminder to (e.g., "group:xxx" or "dm:xxx")
	Enabled       bool
	LastRun       *time.Time
	NextRun       time.Time
	CreatedAt     time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate reminders: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prompt TEXT NOT NULL,
			schedule_type TEXT NOT NULL,
			schedule_value TEXT NOT NULL,
			timezone TEXT DEFAULT 'America/Los_Angeles',
			recipient TEXT DEFAULT '',
			enabled INTEGER DEFAULT 1,
			last_run DATETIME,
			next_run DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_reminders_next_run ON reminders(next_run);
		CREATE INDEX IF NOT EXISTS idx_reminders_enabled ON reminders(enabled);
	`)
	if err != nil {
		return err
	}

	s.db.Exec(`ALTER TABLE reminders ADD COLUMN recipient TEXT DEFAULT ''`)
	return nil
}

func (s *Store) Create(r *Reminder) error {
	if r.Timezone == "" {
		r.Timezone = "America/Los_Angeles"
	}

	nextRun, err := CalculateNextRun(r.ScheduleType, r.ScheduleValue, r.Timezone, time.Now())
	if err != nil {
		return fmt.Errorf("calculate next run: %w", err)
	}
	r.NextRun = nextRun

	result, err := s.db.Exec(
		`INSERT INTO reminders (prompt, schedule_type, schedule_value, timezone, recipient, enabled, next_run)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Prompt, r.ScheduleType, r.ScheduleValue, r.Timezone, r.Recipient, r.Enabled, r.NextRun,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	r.ID = id

	return nil
}

func (s *Store) List() ([]Reminder, error) {
	rows, err := s.db.Query(`
		SELECT id, prompt, schedule_type, schedule_value, timezone, recipient, enabled, last_run, next_run, created_at
		FROM reminders
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var lastRun sql.NullTime
		var recipient sql.NullString
		if err := rows.Scan(&r.ID, &r.Prompt, &r.ScheduleType, &r.ScheduleValue,
			&r.Timezone, &recipient, &r.Enabled, &lastRun, &r.NextRun, &r.CreatedAt); err != nil {
			return nil, err
		}
		if lastRun.Valid {
			r.LastRun = &lastRun.Time
		}
		if recipient.Valid {
			r.Recipient = recipient.String
		}
		reminders = append(reminders, r)
	}

	return reminders, rows.Err()
}

func (s *Store) GetByID(id int64) (*Reminder, error) {
	var r Reminder
	var lastRun sql.NullTime
	var recipient sql.NullString

	err := s.db.QueryRow(`
		SELECT id, prompt, schedule_type, schedule_value, timezone, recipient, enabled, last_run, next_run, created_at
		FROM reminders WHERE id = ?
	`, id).Scan(&r.ID, &r.Prompt, &r.ScheduleType, &r.ScheduleValue,
		&r.Timezone, &recipient, &r.Enabled, &lastRun, &r.NextRun, &r.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		r.LastRun = &lastRun.Time
	}
	if recipient.Valid {
		r.Recipient = recipient.String
	}

	return &r, nil
}

func (s *Store) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM reminders WHERE id = ?", id)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("reminder %d not found", id)
	}

	return nil
}

func (s *Store) SetEnabled(id int64, enabled bool) error {
	result, err := s.db.Exec("UPDATE reminders SET enabled = ? WHERE id = ?", enabled, id)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("reminder %d not found", id)
	}

	return nil
}

func (s *Store) ListDue() ([]Reminder, error) {
	rows, err := s.db.Query(`
		SELECT id, prompt, schedule_type, schedule_value, timezone, recipient, enabled, last_run, next_run, created_at
		FROM reminders
		WHERE enabled = 1 AND next_run <= CURRENT_TIMESTAMP
		ORDER BY next_run ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var lastRun sql.NullTime
		var recipient sql.NullString
		if err := rows.Scan(&r.ID, &r.Prompt, &r.ScheduleType, &r.ScheduleValue,
			&r.Timezone, &recipient, &r.Enabled, &lastRun, &r.NextRun, &r.CreatedAt); err != nil {
			return nil, err
		}
		if lastRun.Valid {
			r.LastRun = &lastRun.Time
		}
		if recipient.Valid {
			r.Recipient = recipient.String
		}
		reminders = append(reminders, r)
	}

	return reminders, rows.Err()
}

func (s *Store) MarkExecuted(id int64) error {
	r, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("reminder %d not found", id)
	}

	now := time.Now()

	if r.ScheduleType == "once" {
		_, err := s.db.Exec(
			"UPDATE reminders SET last_run = ?, enabled = 0 WHERE id = ?",
			now, id,
		)
		return err
	}

	nextRun, err := CalculateNextRun(r.ScheduleType, r.ScheduleValue, r.Timezone, now)
	if err != nil {
		return fmt.Errorf("calculate next run: %w", err)
	}

	_, err = s.db.Exec(
		"UPDATE reminders SET last_run = ?, next_run = ? WHERE id = ?",
		now, nextRun, id,
	)
	return err
}

func CalculateNextRun(scheduleType, scheduleValue, timezone string, from time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := from.In(loc)

	switch scheduleType {
	case "once":
		return parseOnce(scheduleValue, loc)

	case "daily":
		return parseDaily(scheduleValue, now, loc)

	case "hourly":
		return parseHourly(scheduleValue, now, loc)

	case "interval":
		return parseInterval(scheduleValue, now)

	case "cron":
		return parseCron(scheduleValue, now)

	default:
		return time.Time{}, fmt.Errorf("unknown schedule type: %s", scheduleType)
	}
}

func parseOnce(value string, loc *time.Location) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, value, loc); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid once format: %s (expected YYYY-MM-DDTHH:MM)", value)
}

func parseDaily(value string, now time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid daily format: %s (expected HH:MM)", value)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, fmt.Errorf("invalid hour: %s", parts[0])
	}

	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 || min > 59 {
		return time.Time{}, fmt.Errorf("invalid minute: %s", parts[1])
	}

	next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}

	return next, nil
}

func parseHourly(value string, now time.Time, loc *time.Location) (time.Time, error) {
	min, err := strconv.Atoi(value)
	if err != nil || min < 0 || min > 59 {
		return time.Time{}, fmt.Errorf("invalid minute: %s (expected 0-59)", value)
	}

	next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), min, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(time.Hour)
	}

	return next, nil
}

func parseInterval(value string, now time.Time) (time.Time, error) {
	dur, err := time.ParseDuration(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid interval: %s (expected duration like 30m, 2h)", value)
	}

	if dur <= 0 {
		return time.Time{}, fmt.Errorf("interval must be positive: %s", value)
	}

	return now.Add(dur), nil
}

func parseCron(value string, now time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %s (%w)", value, err)
	}

	return schedule.Next(now), nil
}

func ParseSchedule(schedule string) (scheduleType, scheduleValue string, err error) {
	parts := strings.SplitN(schedule, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid schedule format: %s (expected type:value)", schedule)
	}

	scheduleType = parts[0]
	scheduleValue = parts[1]

	switch scheduleType {
	case "once", "daily", "hourly", "interval", "cron":
		return scheduleType, scheduleValue, nil
	default:
		return "", "", fmt.Errorf("unknown schedule type: %s", scheduleType)
	}
}
