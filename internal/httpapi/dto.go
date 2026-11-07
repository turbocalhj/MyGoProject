package httpapi

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/turbocalhj/MyGoProject/internal/shortener"
	"github.com/turbocalhj/MyGoProject/internal/store"
)

type createLinkRequest struct {
	URL       string     `json:"url"`
	Alias     string     `json:"alias"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type updateLinkRequest struct {
	TargetURL *string         `json:"target_url"`
	ExpiresAt json.RawMessage `json:"expires_at"`
	Enabled   *bool           `json:"enabled"`
}

func (r updateLinkRequest) toInput() (shortener.UpdateInput, error) {
	in := shortener.UpdateInput{TargetURL: r.TargetURL, Enabled: r.Enabled}
	if len(r.ExpiresAt) == 0 {
		return in, nil
	}
	if string(r.ExpiresAt) == "null" {
		in.ClearExpires = true
		return in, nil
	}
	var expiresAt time.Time
	if err := json.Unmarshal(r.ExpiresAt, &expiresAt); err != nil {
		return in, fmt.Errorf("expires_at must be an RFC3339 timestamp or null")
	}
	in.ExpiresAt = &expiresAt
	return in, nil
}

type linkResponse struct {
	Code         string     `json:"code"`
	ShortURL     string     `json:"short_url"`
	TargetURL    string     `json:"target_url"`
	Enabled      bool       `json:"enabled"`
	Hits         int64      `json:"hits"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastAccessAt *time.Time `json:"last_access_at"`
}

func newLinkResponse(svc *shortener.Service, l store.Link) linkResponse {
	return linkResponse{
		Code:         l.Code,
		ShortURL:     svc.ShortURL(l.Code),
		TargetURL:    l.TargetURL,
		Enabled:      l.Enabled,
		Hits:         l.Hits,
		ExpiresAt:    l.ExpiresAt,
		CreatedAt:    l.CreatedAt,
		UpdatedAt:    l.UpdatedAt,
		LastAccessAt: l.LastAccessAt,
	}
}

type listResponse struct {
	Items    []linkResponse `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}

type statsResponse struct {
	Code         string     `json:"code"`
	Hits         int64      `json:"hits"`
	LastAccessAt *time.Time `json:"last_access_at"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
}
