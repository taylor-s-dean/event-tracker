package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

// SlackResponse is represents a simple message that can be sent in Slack.
type SlackMessageResponse struct {
	Text string `json:"text"`
}

func slackInteractionResponse(url string, message string) {
	requestBodyObject := SlackMessageResponse{Text: message}
	requestBody, err := json.MarshalIndent(&requestBodyObject, "", "  ")

	// Set a context with a 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate the request.
	request, err := http.NewRequestWithContext(
		ctx,
		"POST",
		url,
		bytes.NewBuffer(requestBody),
	)

	// There really isn't anything to do if an error occurs.
	// The user will not receive a message, but we can log it out to help us detect
	// problems.
	if err != nil {
		log.Printf("Failed to create request to respond to Slack interaction with error: %s", err.Error())
		return
	}

	request.Header.Set("Content-Type", "application/json")

	// Perform the request.
	client := &http.Client{}
	response, err := client.Do(request)

	// Again, really nothing to do here other than log the error.
	if err != nil {
		log.Printf("Failed to perform request to respond to Slack interaction with error: %s", err.Error())
		return
	}

	// Make sure the requests was sucessful and log the response if the request failed.
	if response.StatusCode != http.StatusOK {
		dump, _ := httputil.DumpResponse(response, true)
		log.Printf("Got non-success response when responding to slack interaction:\n%s\n", string(dump))
		return
	}
}

// SlackInteractionData is a partial representation of the request payload used to
// parse out some of the fields.
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

// Validate enforces minimum requirements for requests.
func (r *SlackInteractionData) Validate() error {
	if len(r.Actions) == 0 {
		return fmt.Errorf("Must have at least one action")
	} else if len(r.ResponseURL) == 0 {
		return fmt.Errorf("Request is missing the reponse_url")
	}

	return nil
}

func (request *SlackInteractionData) ParseState() (*Event, error) {
	event := Event{EventType: "INCIDENT"}
	var startDate string
	var startTime string
	var endDate string
	var endTime string

	// This loops over every block of actions
	for _, blockInterface := range request.State.Values {
		block, ok := blockInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Bad block object")
		}

		// This loops over action in the block
		for actionName, valueInterface := range block {
			value, ok := valueInterface.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("Bad action object")
			}

			// Determine whether we care about this data and parse it out if we do.
			switch actionName {
			case "description-action":
				event.Notes, ok = value["value"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad description")
				}
			case "metadata-action":
				event.Metadata, ok = value["value"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad metadata")
				}
			case "start-date-action":
				startDate, ok = value["selected_date"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad start date")
				}
			case "start-time-action":
				startTime, ok = value["selected_time"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad start time")
				}
			case "end-date-action":
				endDate, ok = value["selected_date"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad end date")
				}
			case "end-time-action":
				endTime, ok = value["selected_time"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad end time")
				}
			}
		}
	}

	// Generate proper RFC3339 times and use that to populate time.Time and NullTime
	// objects.
	var err error
	event.StartTime, err = time.ParseInLocation(time.RFC3339, fmt.Sprintf("%sT%s:00Z", startDate, startTime), nil)
	if err != nil {
		return nil, fmt.Errorf("Bad start date and/or time")
	}

	err = event.EndTime.UnmarshalJSON([]byte(fmt.Sprintf(`"%sT%s:00Z"`, endDate, endTime)))
	if err != nil {
		return nil, fmt.Errorf("%s -> Bad end date and/or time", err.Error())
	}

	// Perform some final data clean up.
	if err := event.ValidateAndRectify(); err != nil {
		return nil, err
	}

	return &event, nil
}

func (s *server) SlackInteractionHandler(w http.ResponseWriter, r *http.Request) {
	// Get the payload of the request and ensure that it isn't empty.
	requestJSON := r.FormValue("payload")
	if len(requestJSON) == 0 {
		respondWithJSON(w, http.StatusBadRequest, fmt.Errorf("missing JSON payload"), "", nil)
		return
	}

	// Unmarshal the payload.
	request := SlackInteractionData{}
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	// Validate the request and bail if this is not a sumbit action.
	if err := request.Validate(); err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	} else if request.Actions[0].ActionID != "submit-button-action" {
		respondWithJSON(w, http.StatusOK, nil, "", nil)
		return
	}

	// Parse out remaining relevant information from the state.
	event, err := request.ParseState()
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	// Add a row to the DB.
	if err := s.writeToDB(r.Context(), event, true); err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			err,
			"failed to validate request and write to database",
			nil,
		)
		return
	}

	// Generate a Slack message as a response to the user's interaction.
	eventBytes, err := json.MarshalIndent(&event, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal event: %s", err.Error())
	}

	// Send the Slack message asyncronously.
	go slackInteractionResponse(
		request.ResponseURL,
		fmt.Sprintf("Created event with the following parameters: ```%s```", string(eventBytes)),
	)

	// Acknowledge the request.
	respondWithJSON(
		w,
		http.StatusOK,
		nil,
		"",
		nil,
	)
}
