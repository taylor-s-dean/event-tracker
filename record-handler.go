package main

import (
	"encoding/json"
	"net/http"
)

func (s *server) RecordHandler(w http.ResponseWriter, r *http.Request) {
	event := Event{}
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	if err := event.ValidateAndRectify(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	err := s.writeToDB(r.Context(), &event)
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

	respondWithJSON(w, http.StatusOK, err, "", event)
}
