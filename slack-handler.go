package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/gorilla/schema"
)

var (
	messageTemplate = template.Must(template.New("").Parse(`
{
	"blocks": [
		{
			"type": "section",
			"text": {
				"type": "plain_text",
				"text": "Record a site incident by filling out the following data.",
				"emoji": true
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Incident Start Date and Time*"
			}
		},
		{
			"type": "actions",
			"elements": [
				{
					"type": "datepicker",
					"initial_date": "{{.start_date}}",
					"placeholder": {
						"type": "plain_text",
						"text": "Select a date",
						"emoji": true
					},
					"action_id": "start-date-action"
				},
				{
					"type": "timepicker",
					"initial_time": "{{.start_time}}",
					"placeholder": {
						"type": "plain_text",
						"text": "Select time",
						"emoji": true
					},
					"action_id": "start-time-action"
				}
			]
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Incident End Date and Time*\nLeave unchanged if incident should be considered instantaneous."
			}
		},
		{
			"type": "actions",
			"elements": [
				{
					"type": "datepicker",
					"initial_date": "{{.end_date}}",
					"placeholder": {
						"type": "plain_text",
						"text": "Select a date",
						"emoji": true
					},
					"action_id": "end-date-action"
				},
				{
					"type": "timepicker",
					"initial_time": "{{.end_time}}",
					"placeholder": {
						"type": "plain_text",
						"text": "Select time",
						"emoji": true
					},
					"action_id": "end-time-action"
				}
			]
		},
		{
			"type": "input",
			"element": {
				"type": "plain_text_input",
				"action_id": "description-action"
			},
			"label": {
				"type": "plain_text",
				"text": "Description of Incident",
				"emoji": true
			}
		},
		{
			"type": "input",
			"element": {
				"type": "plain_text_input",
				"action_id": "metadata-action"
			},
			"label": {
				"type": "plain_text",
				"text": "Arbitrary Valid JSON Metadata",
				"emoji": true
			}
		},
		{
			"type": "actions",
			"elements": [
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "Submit",
						"emoji": true
					},
					"value": "click_me_123",
					"action_id": "submit-button-action"
				}
			]
		}
	]
}
`))
)

type SlackCommandData struct {
	Command string `schema:"command"`
}

type SlackResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
}

func slackCommandResponse(w http.ResponseWriter, data *SlackCommandData) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	messagePayload := map[string]interface{}{}
	now := time.Now()
	startDate := now.Format("2006-01-02")
	endDate := now.Add(-24 * 365 * time.Hour).Format("2006-01-02")
	startEndTime := now.Format("15:04")
	var message bytes.Buffer
	if err := messageTemplate.Execute(&message, map[string]string{"start_time": startEndTime, "start_date": startDate, "end_time": startEndTime, "end_date": endDate}); err != nil {
		return
	}

	if err := json.Unmarshal(message.Bytes(), &messagePayload); err != nil {
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	encoder.Encode(messagePayload)
}

func slackInteractionResponse(url string, message string, err error, code int) {
	requestBodyObject := SlackResponse{}
	if err != nil {
		requestBodyObject.Text = fmt.Sprintf("Message: %s\nCode: %d\nError: %s", message, code, err.Error())
	} else {
		requestBodyObject.Text = message
		requestBodyObject.ResponseType = "in_channel"
	}

	requestBody, err := json.MarshalIndent(&requestBodyObject, "", "  ")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx,
		"POST",
		url,
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		log.Printf("Failed to create request to respond to Slack interaction with error: %s", err.Error())
		return
	}

	request.Header.Set("Content-Type", "application/json")

	dump, _ := httputil.DumpRequestOut(request, true)
	log.Println(string(dump))

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		log.Printf("Failed to perform request to respond to Slack interaction with error: %s", err.Error())
		return
	}

	dump, _ = httputil.DumpResponse(response, true)
	if response.StatusCode != http.StatusOK {
		log.Printf("Got non-success response when responding to slack interaction")
		return
	}
}

func (s *server) SlackCommandHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	request := SlackCommandData{}
	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)
	if err := decoder.Decode(&request, r.Form); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	slackCommandResponse(w, &request)
	return
}

type SlackInteractionData struct {
	Type string `json:"type"` // expect "block_actions"
	User struct {
		Name string `json:"name"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
	} `json:"actions"`
	State struct {
		Values map[string]interface{} `json:"values"`
	} `json:"state"`
	ResponseURL string `json:"response_url"`
}

func (s *server) SlackInteractionHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	requestJSON := r.FormValue("payload")
	request := SlackInteractionData{}
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	if len(request.Actions) == 0 {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Must have at least one action"), "", nil)
		return
	} else if request.Actions[0].ActionID != "submit-button-action" {
		respondWithJSON(w, http.StatusOK, nil, "", nil)
		return
	} else if len(request.ResponseURL) == 0 {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Request is missing the reponse_url"), "", nil)
		return
	}

	var description string
	var metadata string
	var startDate string
	var startTime string
	var endDate string
	var endTime string

	// This loops over every block of actions
	for _, blockInterface := range request.State.Values {
		block, ok := blockInterface.(map[string]interface{})
		if !ok {
			respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad block object"), "", nil)
			return
		}
		// This loops over action in the block
		for actionName, valueInterface := range block {
			value, ok := valueInterface.(map[string]interface{})
			if !ok {
				respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad action object"), "", nil)
				return
			}

			switch actionName {
			case "description-action":
				description, ok = value["value"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad description"), "", nil)
					return
				}
			case "metadata-action":
				metadata, ok = value["value"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad metadata"), "", nil)
					return
				}
			case "start-date-action":
				startDate, ok = value["selected_date"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad start date"), "", nil)
					return
				}
			case "start-time-action":
				startTime, ok = value["selected_time"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad start time"), "", nil)
					return
				}
			case "end-date-action":
				endDate, ok = value["selected_date"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad end date"), "", nil)
					return
				}
			case "end-time-action":
				endTime, ok = value["selected_time"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad end time"), "", nil)
					return
				}
			}
		}
	}

	startDateTime, err := time.ParseInLocation(time.RFC3339, fmt.Sprintf("%sT%s:00Z", startDate, startTime), nil)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad start date and/or time"), "", nil)
		return
	}

	endDateTime := NullTime{}
	err = json.Unmarshal([]byte(fmt.Sprintf(`"%sT%s:00Z"`, endDate, endTime)), &endDateTime)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("%s -> Bad end date and/or time", err.Error()), "", nil)
		return
	}

	event, code, err := s.validateAndWriteToDB(r.Context(), "INCIDENT", startDateTime, endDateTime, description, metadata, true)
	if err != nil {
		respondWithJSON(
			w,
			code,
			err,
			"failed to validate request and write to database",
			nil,
		)
		return
	}

	eventBytes, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		log.Printf("Failed to unmarshal event: %s", err.Error())
	}

	go slackInteractionResponse(
		request.ResponseURL,
		fmt.Sprintf("Created event with the following parameters: ```%s```", string(eventBytes)),
		nil,
		http.StatusOK,
	)

	respondWithJSON(
		w,
		http.StatusOK,
		nil,
		"",
		nil,
	)
}

type Event struct {
	ID        int64       `json:"id"`
	Type      string      `json:"type"`
	Notes     string      `json:"notes"`
	StartTime time.Time   `json:"start_time"`
	EndTime   NullTime    `json:"end_time"`
	Metadata  interface{} `json:"metadata"`
}

func (s *server) validateAndWriteToDB(ctx context.Context, eventType string, startTime time.Time, endTime NullTime, notes string, metadata string, dryRun bool) (*Event, int, error) {
	if len(eventType) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("event_type parameter is required")
	} else if len(notes) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("notes parameter is required")
	}

	if !json.Valid([]byte(metadata)) {
		return nil, http.StatusBadRequest, fmt.Errorf("metadata must be valid json")
	}

	event := Event{
		ID:        rand.Int63(),
		Type:      eventType,
		Notes:     notes,
		StartTime: startTime,
		EndTime:   endTime,
		Metadata:  metadata,
	}

	if event.StartTime.IsZero() {
		event.StartTime = time.Now()
	}

	if event.EndTime.Valid && !event.EndTime.Time.After(event.StartTime) {
		event.EndTime.Valid = false
	}

	if !dryRun {
		_, err := s.db.ExecContext(ctx, `
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
`, event.ID, event.Type, event.StartTime, event.EndTime, event.Notes, event.Metadata)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}

	} else {
		endTimeBytes, _ := json.Marshal(event.EndTime)

		log.Printf(`
INSERT INTO events (
	id,
	event_type,
	start_time,
	end_time,
	notes,
	metadata
) VALUES (
	%d,
	'%s',
	'%s',
	'%s',
	'%s',
	'%s'
)
`, event.ID, event.Type, event.StartTime.Format(time.RFC3339), string(endTimeBytes), event.Notes, event.Metadata)
	}

	return &event, http.StatusOK, nil
}

type NullTime struct {
	sql.NullTime
}

func (n *NullTime) UnmarshalJSON(data []byte) error {
	var t *time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}

	if t != nil && !t.IsZero() {
		n.Valid = true
		n.Time = *t
	} else {
		n.Valid = false
	}

	return nil
}

func (n *NullTime) MarshalJSON() ([]byte, error) {
	if n.Valid {
		return json.Marshal(n.Time)
	}

	return json.Marshal(nil)
}
