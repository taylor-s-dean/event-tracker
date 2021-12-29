package main

import (
	"database/sql"
	"encoding/json"
	"time"
)

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
		n.Time = time.Time{}
	}

	return nil
}

func (n *NullTime) MarshalJSON() ([]byte, error) {
	if n.Valid {
		return json.Marshal(n.Time)
	}

	return json.Marshal(nil)
}
