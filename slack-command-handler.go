package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
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

// SlackCommandData is the request body.
type SlackCommandData struct {
	Command string `schema:"command"`
}

func slackCommandResponse(w http.ResponseWriter, data *SlackCommandData) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Determine the start/end date/time for the message template.
	now := time.Now()
	startDate := now.Format("2006-01-02")
	endDate := now.Add(-24 * 365 * time.Hour).Format("2006-01-02")
	startEndTime := now.Format("15:04")

	// Execute the template to replace the values with the newly calculated start/end
	// date/time.
	var message bytes.Buffer
	if err := messageTemplate.Execute(&message, map[string]string{
		"start_time": startEndTime,
		"start_date": startDate,
		"end_time":   startEndTime,
		"end_date":   endDate,
	}); err != nil {
		return
	}

	// Populate the message payload with the interpolated JSON template.
	var messagePayload interface{}
	if err := json.Unmarshal(message.Bytes(), &messagePayload); err != nil {
		return
	}

	encoder := json.NewEncoder(w)
	encoder.Encode(messagePayload)
}

func (s *server) SlackCommandHandler(w http.ResponseWriter, r *http.Request) {
	// We always have to parse the form before accessing the data.
	if err := r.ParseForm(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	// Unmarshal the form data into the struct.
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
