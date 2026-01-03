package reminder

import (
	"context"
	"fmt"
	"log"
	"time"
)

type ExecuteFunc func(recipient, prompt string) (string, error)
type SendFunc func(recipient, message string) error

type Scheduler struct {
	store       *Store
	executeFunc ExecuteFunc
	sendFunc    SendFunc
	interval    time.Duration
	debug       bool
}

func NewScheduler(store *Store, executeFunc ExecuteFunc, sendFunc SendFunc, debug bool) *Scheduler {
	return &Scheduler{
		store:       store,
		executeFunc: executeFunc,
		sendFunc:    sendFunc,
		interval:    1 * time.Minute,
		debug:       debug,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	log.Println("[reminder] scheduler started")

	for {
		select {
		case <-ctx.Done():
			log.Println("[reminder] scheduler stopped")
			return
		case <-ticker.C:
			s.checkDueReminders()
		}
	}
}

func (s *Scheduler) checkDueReminders() {
	reminders, err := s.store.ListDue()
	if err != nil {
		log.Printf("[reminder] error listing due reminders: %v", err)
		return
	}

	for _, r := range reminders {
		s.executeReminder(r)
	}
}

func (s *Scheduler) executeReminder(r Reminder) {
	if s.debug {
		log.Printf("[reminder] executing reminder %d (recipient: %s)", r.ID, r.Recipient)
	}

	result, err := s.executeFunc(r.Recipient, r.Prompt)
	if err != nil {
		log.Printf("[reminder] error executing reminder %d: %v", r.ID, err)
		result = fmt.Sprintf("Reminder %d failed: %v", r.ID, err)
	}

	message := result

	if err := s.sendFunc(r.Recipient, message); err != nil {
		log.Printf("[reminder] error sending reminder %d: %v", r.ID, err)
		return
	}

	if err := s.store.MarkExecuted(r.ID); err != nil {
		log.Printf("[reminder] error marking reminder %d as executed: %v", r.ID, err)
	}

	if s.debug {
		log.Printf("[reminder] reminder %d executed successfully", r.ID)
	}
}

func (s *Scheduler) RunNow(id int64) error {
	r, err := s.store.GetByID(id)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("reminder %d not found", id)
	}

	s.executeReminder(*r)
	return nil
}
