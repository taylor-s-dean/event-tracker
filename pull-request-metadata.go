package main

import (
	"fmt"
)

type PullRequestMetadata struct {
	Repository    string `json:"repository"`
	PullRequestID uint32 `json:"pull_request_id"`
	URL           string `json:"url"`
}

func (m *PullRequestMetadata) Validate() error {
	if len(m.Repository) == 0 {
		return fmt.Errorf("\"repository\" should be non-empty")
	} else if m.PullRequestID == 0 {
		return fmt.Errorf("\"pull_request_id\" must be non-zero")
	} else if len(m.URL) == 0 {
		return fmt.Errorf("\"url\" should be non-empty")
	}

	return nil
}
