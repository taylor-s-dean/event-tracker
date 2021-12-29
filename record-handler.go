package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RecordData struct {
	EventType string      `json:"event_type"`
	Notes     string      `json:"notes"`
	StartTime time.Time   `json:"start_time"`
	EndTime   NullTime    `json:"end_time"`
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

	event, code, err := s.validateAndWriteToDB(r.Context(), request.EventType, request.StartTime, request.EndTime, request.Notes, string(metadataBytes), false)
	if err != nil {
		respondWithJSON(
			w,
			code,
			err,
			"failed to write to database",
			nil,
		)
		return
	}

	respondWithJSON(w, http.StatusOK, err, "", event)
}
