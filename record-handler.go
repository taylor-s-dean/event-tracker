package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

type RecordData struct {
	EventType string      `json:"event_type"`
	Notes     string      `json:"notes"`
	StartTime time.Time   `json:"start_time"`
	EndTime   time.Time   `json:"end_time"`
	Metadata  interface{} `json:"metadata"`
}

type RecordResponse struct {
	ID      int64       `json:"id"`
	Request *RecordData `json:"request"`
}

func (d *RecordData) Validate() error {
	if len(d.EventType) == 0 {
		return fmt.Errorf("event_type parameter is required")
	} else if len(d.Notes) == 0 {
		return fmt.Errorf("notes parameter is required")
	}
	return nil
}

func (s *server) RecordHandler(w http.ResponseWriter, r *http.Request) {
	request := RecordData{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	if err := request.Validate(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	metadataBytes, err := json.Marshal(request.Metadata)
	if err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			err,
			"failed to marshal metadata",
			nil,
		)
		return
	}

	endTime := sql.NullTime{}
	if !request.EndTime.IsZero() {
		endTime.Time = request.EndTime
		endTime.Valid = true
	}

	if request.StartTime.IsZero() {
		request.StartTime = time.Now()
	}

	id := rand.Int63()
	_, err = s.db.ExecContext(r.Context(), `
INSERT INTO events (
	id,
	event_type,
	start_time,
	end_time,
	notes,
	metadata
) VALUES (
	?,
	?,
	?,
	?,
	?,
	?
)
`, id, request.EventType, request.StartTime, endTime, request.Notes, string(metadataBytes))
	if err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			err,
			"failed to write to database",
			nil,
		)
		return
	}

	respondWithJSON(w, http.StatusOK, err, "", RecordResponse{ID: id, Request: &request})
}
