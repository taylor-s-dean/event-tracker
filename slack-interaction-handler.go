package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"makeshift.dev/event-tracker/slack"
)

const (
	slackPostMessageURL = "https://slack.com/api/chat.postMessage"
)

func (s *server) slackInteractionResponse(channel string, message string) {
	request := slack.NewChatPostMessageRequest(channel)
	request.Text = message
	if _, err := s.SlackClient.ChatPostMessage(request); err != nil {
		log.Printf("Failed to post message with error: %s", err.Error())
	}
}

func (s *server) slackInteractionEphemeralResponse(url string, message string) {
	requestBody, err := json.MarshalIndent(map[string]string{"text": message}, "", "  ")

	// Set a context with a 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate the request.
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewBuffer(requestBody),
	)

	if err != nil {
		log.Printf("Failed to generate HTTP request with error: %s", err.Error())
	}

	httpClient := &http.Client{}
	_, err = httpClient.Do(httpRequest)
	if err != nil {
		log.Printf("Failed to post message with error: %s", err.Error())
	}
}

// SlackInteractionData is a partial representation of the request payload used to
// parse out some of the fields.
type SlackInteractionData struct {
	Type string `json:"type"` // expect "block_actions"
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
	} `json:"actions"`
	State struct {
		Values map[string]interface{} `json:"values"`
	} `json:"state"`
	ResponseURL string `json:"response_url"`
	Channel     struct {
		ID string `json:"id"`
	} `json:"channel"`
}

// Validate enforces minimum requirements for requests.
func (r *SlackInteractionData) Validate() error {
	if len(r.Actions) == 0 {
		return fmt.Errorf("Must have at least one action")
	} else if len(r.ResponseURL) == 0 {
		return fmt.Errorf("Request is missing the reponse_url")
	} else if len(r.Channel.ID) == 0 {
		return fmt.Errorf("Request is missing the channel id")
	}

	return nil
}

func (request *SlackInteractionData) ParseState(tzOffsetSeconds int) (*Event, error) {
	event := Event{EventType: "INCIDENT", DryRun: true}
	var startDate string
	var startTime string
	var endDate string
	var endTime string
	var postmortem string

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
			case "postmortem-action":
				postmortem, ok = value["value"].(string)
				if !ok {
					return nil, fmt.Errorf("Bad postmortem")
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
			case "checkbox-action":
				selectedOptions, ok := value["selected_options"].([]interface{})
				if !ok {
					return nil, fmt.Errorf("Bad checkbox selected options")
				}
				for _, optionObj := range selectedOptions {
					option, ok := optionObj.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("Bad checkbox option")
					}
					value, ok := option["value"].(string)
					if !ok {
						return nil, fmt.Errorf("Bad checkbox option value")
					}
					if value == "value-0" {
						event.DryRun = false
						break
					}
				}
			}
		}
	}

	// Generate proper RFC3339 times and use that to populate time.Time and NullTime
	// objects.
	sign := "+"
	if tzOffsetSeconds < 0 {
		sign = "-"
	}
	tzOffset := time.Duration(math.Abs(float64(tzOffsetSeconds))) * time.Second
	tzOffset = tzOffset.Round(time.Minute)
	hours := int(tzOffset.Hours())
	minutes := int(tzOffset.Minutes()) - 60*hours
	var err error
	event.StartTime, err = time.Parse(time.RFC3339, fmt.Sprintf("%sT%s:00%s%02d:%02d", startDate, startTime, sign, hours, minutes))
	if err != nil {
		return nil, fmt.Errorf("Bad start date and/or time")
	}

	log.Printf("Location: %s", event.StartTime.Location().String())
	err = event.EndTime.UnmarshalJSON([]byte(fmt.Sprintf(`"%sT%s:00%s%02d:%02d"`, endDate, endTime, sign, hours, minutes)))
	if err != nil {
		return nil, fmt.Errorf("Failed to parse timestamp for end time: %w", err)
	}

	event.Metadata = map[string]string{"postmortem": postmortem}

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

	usersInfoRequest := slack.NewUsersInfoRequest(request.User.ID)
	usersInfoResponse, err := s.SlackClient.UsersInfo(usersInfoRequest)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, err, "", nil)
		return
	}

	// Parse out remaining relevant information from the state.
	event, err := request.ParseState(usersInfoResponse.User.TZOffset)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, err, "", nil)
		return
	}

	// Add a row to the DB.
	if err := s.writeToDB(r.Context(), event); err != nil {
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
	message := fmt.Sprintf("<@%s> created event with the following parameters: ```%s```", request.User.ID, string(eventBytes))
	if !event.DryRun {
		go s.slackInteractionResponse(request.Channel.ID, message)
	} else {
		go s.slackInteractionEphemeralResponse(request.ResponseURL, message)
	}

	// Acknowledge the request.
	respondWithJSON(
		w,
		http.StatusOK,
		nil,
		"",
		nil,
	)
}
