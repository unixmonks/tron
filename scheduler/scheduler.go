package scheduler

import (
	"context"
	"log"
	"time"
)

type SummaryFunc func() (string, error)
type SendFunc func(message string) error

type Scheduler struct {
	hour        int
	location    *time.Location
	summaryFunc SummaryFunc
	sendFunc    SendFunc
	lastSent    time.Time
}

func NewScheduler(hour int, summaryFunc SummaryFunc, sendFunc SendFunc) (*Scheduler, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}

	return &Scheduler{
		hour:        hour,
		location:    loc,
		summaryFunc: summaryFunc,
		sendFunc:    sendFunc,
	}, nil
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	log.Printf("Scheduler started, will send daily summary at %02d:00 PDT", s.hour)

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopped")
			return
		case <-ticker.C:
			s.checkAndSend()
		}
	}
}

func (s *Scheduler) checkAndSend() {
	now := time.Now().In(s.location)

	if now.Hour() != s.hour {
		return
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	if !s.lastSent.Before(today) {
		return
	}

	log.Println("Sending daily summary...")

	summary, err := s.summaryFunc()
	if err != nil {
		log.Printf("Error generating summary: %v", err)
		return
	}

	if err := s.sendFunc(summary); err != nil {
		log.Printf("Error sending summary: %v", err)
		return
	}

	s.lastSent = now
	log.Println("Daily summary sent successfully")
}

func (s *Scheduler) SendNow() error {
	summary, err := s.summaryFunc()
	if err != nil {
		return err
	}
	return s.sendFunc(summary)
}
