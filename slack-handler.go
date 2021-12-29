package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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

	event := Event{EventType: "INCIDENT"}
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
				event.Notes, ok = value["value"].(string)
				if !ok {
					respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad description"), "", nil)
					return
				}
			case "metadata-action":
				event.Metadata, ok = value["value"].(string)
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

	var err error
	event.StartTime, err = time.ParseInLocation(time.RFC3339, fmt.Sprintf("%sT%s:00Z", startDate, startTime), nil)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("Bad start date and/or time"), "", nil)
		return
	}

	err = event.EndTime.UnmarshalJSON([]byte(fmt.Sprintf(`"%sT%s:00Z"`, endDate, endTime)))
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("%s -> Bad end date and/or time", err.Error()), "", nil)
		return
	}

	if err := event.ValidateAndRectify(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	if err := s.writeToDB(r.Context(), &event, true); err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			err,
			"failed to validate request and write to database",
			nil,
		)
		return
	}

	eventBytes, err := json.MarshalIndent(&event, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal event: %s", err.Error())
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

func (s *server) writeToDB(ctx context.Context, event *Event, dryRun bool) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata to []byte")
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
`, event.ID, event.EventType, event.StartTime, event.EndTime, event.Notes, metadata)
		if err != nil {
			return err
		}

	} else {
		endTimeBytes, _ := event.EndTime.MarshalJSON()

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
`, event.ID, event.EventType, event.StartTime.Format(time.RFC3339), string(endTimeBytes), event.Notes, event.Metadata)
	}

	return nil
}
