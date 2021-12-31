package main

import (
	"fmt"
	"math/rand"
	"time"
)

// EventData is the Go representation of the request JSON object.
type Event struct {
	ID        int64       `json:"ID"`
	EventType string      `json:"event_type"`
	Notes     string      `json:"notes"`
	StartTime time.Time   `json:"start_time"`
	EndTime   NullTime    `json:"end_time"`
	Metadata  interface{} `json:"metadata"`
	DryRun    bool        `json:"-"`
}

func (d *Event) ValidateAndRectify() error {
	if len(d.EventType) == 0 {
		return fmt.Errorf("event_type parameter is required")
	} else if len(d.Notes) == 0 {
		return fmt.Errorf("notes parameter is required")
	}

	if d.StartTime.IsZero() {
		d.StartTime = time.Now()
	}

	if d.EndTime.Valid && !d.EndTime.Time.After(d.StartTime) {
		d.EndTime.Valid = false
	}

	d.ID = rand.Int63()

	return nil
}
