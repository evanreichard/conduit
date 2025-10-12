package store

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

type TunnelRecord struct {
	ID         uuid.UUID
	Time       time.Time
	URL        *url.URL
	Method     string
	Status     int
	SourceAddr string

	RequestHeaders  http.Header
	RequestBodyType string
	RequestBody     []byte

	ResponseHeaders  http.Header
	ResponseBodyType string
	ResponseBody     []byte
}

func (tr *TunnelRecord) MarshalJSON() ([]byte, error) {
	type Alias TunnelRecord
	return json.Marshal(&struct {
		*Alias
		URL  string `json:"URL"`
		Time string `json:"Time"`
	}{
		Alias: (*Alias)(tr),
		URL:   tr.URL.String(),
		Time:  tr.Time.Format(time.RFC3339),
	})
}

func (tr *TunnelRecord) UnmarshalJSON(data []byte) error {
	type Alias TunnelRecord
	aux := &struct {
		*Alias
		URL  string `json:"URL"`
		Time string `json:"Time"`
	}{
		Alias: (*Alias)(tr),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	parsedURL, err := url.Parse(aux.URL)
	if err != nil {
		return err
	}
	tr.URL = parsedURL

	parsedTime, err := time.Parse(time.RFC3339, aux.Time)
	if err != nil {
		return err
	}
	tr.Time = parsedTime

	return nil
}
