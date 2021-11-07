package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"
)

type PullRequestData struct {
	Action      string `json:"action"`
	Number      int32  `json:"number"`
	PullRequest struct {
		URL       string    `json:"html_url"`
		Merged    bool      `json:"merged"`
		Title     string    `json:"title"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type PullRequestResponse struct {
	ID int64 `json:"id"`
}

func (s *server) PullRequestHandler(w http.ResponseWriter, r *http.Request) {
	request := PullRequestData{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	metadataBytes, err := json.Marshal(request)
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

	if request.Action != "closed" || !request.PullRequest.Merged {
		respondWithJSON(w, http.StatusOK, nil, "", request)
		return
	}

	response := PullRequestResponse{ID: rand.Int63()}
	_, err = s.db.ExecContext(r.Context(), `
INSERT INTO events (
	id,
	event_type,
	start_time,
	notes,
	metadata
) VALUES (
	?,
	?,
	?,
	?,
	?
)
`, response.ID, "PULL REQUEST", request.PullRequest.UpdatedAt, request.PullRequest.Title, string(metadataBytes))
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

	respondWithJSON(w, http.StatusOK, err, "", response)
}
