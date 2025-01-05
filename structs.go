package main

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type User struct {
	id uint
	ip string
}

// Represents a vote sent to a user.
// Contains a random UUID to prevent vote manipulation by modifying responses.
type VoteOptions struct {
	startTime time.Time
	A         string `json:"a"`
	B         string `json:"b"`
}

type VoteObject map[int]bool

func (s *VoteObject) Scan(src interface{}) error {
	if src == nil {
		return sql.ErrNoRows
	}
	switch data := src.(type) {
	case string:
		return json.Unmarshal([]byte(data), &s)
	case []byte:
		return json.Unmarshal(data, &s)
	default:
		return fmt.Errorf("cannot scan type %t into Map", src)
	}
}

func (t VoteObject) Value() (driver.Value, error) {
	b, err := json.Marshal(t)
	return string(b), err
}
