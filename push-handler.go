package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type PushData struct {
	Ref        string `json:"ref"`
	HeadCommit struct {
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		URL       string    `json:"url"`
		Author    struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
		Committer struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"committer"`
		Added    []string `json:"added"`
		Removed  []string `json:"removed"`
		Modified []string `json:"modified"`
	} `json:"head_commit"`
	Repository struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		MasterBranch  string `json:"master_branch"`
	} `json:"repository"`
	Pusher struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"pusher"`
}

type PushResponse struct {
	ID int64 `json:"id"`
}

func (s *server) PushHandler(w http.ResponseWriter, r *http.Request) {
	request := PushData{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	if !strings.Contains(request.Ref, request.Repository.DefaultBranch) &&
		!strings.Contains(request.Ref, request.Repository.MasterBranch) {
		respondWithJSON(w, http.StatusOK, nil, "", request)
		return
	}

	event := &Event{
		EventType: "PUSH",
		StartTime: request.HeadCommit.Timestamp,
		Notes:     request.HeadCommit.Message,
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
