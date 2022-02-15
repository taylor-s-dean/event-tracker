package main

import (
	"encoding/json"
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
		Body      string    `json:"body"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
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

	if request.Action != "closed" || !request.PullRequest.Merged {
		respondWithJSON(w, http.StatusOK, nil, "", request)
		return
	}

	event := &Event{
		EventType: "PULL REQUEST",
		StartTime: request.PullRequest.UpdatedAt,
		Notes:     request.PullRequest.Title,
		Metadata:  request,
	}

	if err := s.writeToDBAndLog(r.Context(), event); err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			err,
			"failed to write to database",
			nil,
		)
		return
	}

	respondWithJSON(w, http.StatusOK, nil, "", event)
}
