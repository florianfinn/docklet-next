// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original Plex and Overseerr response projections; no upstream code copied.

package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	dto "github.com/florianfinn/docklet-next/types/widget"
)

func text(s string) string {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}
func count(n int) int { return min(max(n, 0), 1_000_000) }

func fetchPlex(ctx context.Context, f *fetchClient) (*dto.PlexData, error) {
	var payload struct {
		MediaContainer *struct {
			Size     *int `json:"size"`
			Metadata []struct {
				Title            *string `json:"title"`
				Type             string  `json:"type"`
				GrandparentTitle string  `json:"grandparentTitle"`
				ParentTitle      string  `json:"parentTitle"`
				ParentIndex      int     `json:"parentIndex"`
				Index            int     `json:"index"`
				ViewOffset       float64 `json:"viewOffset"`
				Duration         float64 `json:"duration"`
				User             *struct {
					Title *string `json:"title"`
				} `json:"User"`
				Player *struct {
					Product string `json:"product"`
					Title   string `json:"title"`
				} `json:"Player"`
				Media []struct {
					VideoResolution string          `json:"videoResolution"`
					AudioCodec      string          `json:"audioCodec"`
					Selected        json.RawMessage `json:"selected"`
				} `json:"Media"`
				TranscodeSession *struct {
					VideoDecision string `json:"videoDecision"`
					AudioDecision string `json:"audioDecision"`
					Height        int    `json:"height"`
					AudioCodec    string `json:"audioCodec"`
				} `json:"TranscodeSession"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := f.request(ctx, "GET", "/status/sessions", &payload); err != nil {
		return nil, err
	}
	m := payload.MediaContainer
	if m == nil || m.Size == nil || *m.Size < 0 || (*m.Size > 0 && len(m.Metadata) == 0) {
		return nil, dto.Failure(dto.BadResponse)
	}
	out := &dto.PlexData{Streams: count(*m.Size), Sessions: []dto.PlexSession{}}
	for _, s := range m.Metadata[:min(20, len(m.Metadata))] {
		if s.Title == nil || s.User == nil || s.User.Title == nil || s.Player == nil {
			return nil, dto.Failure(dto.BadResponse)
		}
		item := dto.PlexSession{User: text(*s.User.Title), Title: text(*s.Title), Device: text(strings.TrimSpace(s.Player.Product + " " + s.Player.Title))}
		if s.Type == "episode" {
			item.Title = text(s.GrandparentTitle + " – " + *s.Title)
			item.Subtitle = fmt.Sprintf("S%d E%d", count(s.ParentIndex), count(s.Index))
		}
		if s.Type == "track" {
			item.Subtitle = text(s.ParentTitle)
		}
		if s.Duration > 0 && !math.IsNaN(s.Duration) && !math.IsInf(s.Duration, 0) {
			item.Progress = min(max(s.ViewOffset/s.Duration, 0), 1)
		}
		if len(s.Media) > 0 {
			media := s.Media[0]
			item.Quality = media.VideoResolution
			if s.Type == "track" {
				item.Quality = media.AudioCodec
			}
		}
		if t := s.TranscodeSession; t != nil {
			item.Transcode = t.VideoDecision == "transcode" || t.AudioDecision == "transcode"
			target := ""
			if t.Height > 0 {
				target = strconv.Itoa(count(t.Height))
			}
			if target == "" {
				for _, media := range s.Media {
					if string(media.Selected) == "true" || string(media.Selected) == "1" {
						target = media.VideoResolution
						break
					}
				}
			}
			if s.Type == "track" {
				target = t.AudioCodec
			}
			if target != "" && target != item.Quality {
				item.Quality += " → " + target
			}
		}
		item.Quality = text(item.Quality)
		if item.Transcode {
			out.Transcodes++
		}
		out.Sessions = append(out.Sessions, item)
	}
	return out, nil
}

type titleKey struct {
	Revision   int64
	Generation uint64
	Media      string
	ID         int64
}
type titleEntry struct {
	Title string
	At    time.Time
}
type titleCache struct {
	mu      sync.Mutex
	entries map[titleKey]titleEntry
}

func newTitles() *titleCache { return &titleCache{entries: map[titleKey]titleEntry{}} }
func (c *titleCache) get(k titleKey, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	return e.Title, ok && now.Sub(e.At) < 24*time.Hour
}
func (c *titleCache) put(k titleKey, title string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= 1024 {
		clear(c.entries)
	}
	c.entries[k] = titleEntry{Title: title, At: now}
}

func fetchOverseerr(ctx context.Context, f *fetchClient, revision int64, generation uint64, titles *titleCache, now time.Time) (*dto.OverseerrData, error) {
	var counts struct {
		Pending   *int `json:"pending"`
		Approved  *int `json:"approved"`
		Available *int `json:"available"`
	}
	if err := f.request(ctx, "GET", "/api/v1/request/count", &counts); err != nil {
		return nil, err
	}
	if counts.Pending == nil || counts.Approved == nil || counts.Available == nil || *counts.Pending < 0 || *counts.Approved < 0 || *counts.Available < 0 {
		return nil, dto.Failure(dto.BadResponse)
	}
	var list struct {
		Results *[]struct {
			ID          int64   `json:"id"`
			Type        string  `json:"type"`
			CreatedAt   *string `json:"createdAt"`
			RequestedBy *struct {
				DisplayName  string `json:"displayName"`
				Username     string `json:"username"`
				PlexUsername string `json:"plexUsername"`
				Email        string `json:"email"`
			} `json:"requestedBy"`
			Media *struct {
				TMDBID    int64  `json:"tmdbId"`
				MediaType string `json:"mediaType"`
			} `json:"media"`
		} `json:"results"`
	}
	if err := f.request(ctx, "GET", "/api/v1/request?take=5&filter=pending&sort=added", &list); err != nil {
		return nil, err
	}
	if list.Results == nil {
		return nil, dto.Failure(dto.BadResponse)
	}
	out := &dto.OverseerrData{Pending: count(*counts.Pending), Approved: count(*counts.Approved), Available: count(*counts.Available), Requests: []dto.MediaRequest{}}
	for _, r := range (*list.Results)[:min(5, len(*list.Results))] {
		if r.ID <= 0 || r.Media == nil || r.Media.TMDBID <= 0 || r.RequestedBy == nil || r.CreatedAt == nil {
			return nil, dto.Failure(dto.BadResponse)
		}
		media := r.Type
		if media == "" {
			media = r.Media.MediaType
		}
		if media != "movie" && media != "tv" {
			return nil, dto.Failure(dto.BadResponse)
		}
		if _, err := time.Parse(time.RFC3339Nano, *r.CreatedAt); err != nil {
			return nil, dto.Failure(dto.BadResponse)
		}
		key := titleKey{Revision: revision, Generation: generation, Media: media, ID: r.Media.TMDBID}
		title, ok := titles.get(key, now)
		if !ok {
			var detail struct {
				Title *string `json:"title"`
				Name  *string `json:"name"`
			}
			if err := f.request(ctx, "GET", "/api/v1/"+media+"/"+strconv.FormatInt(r.Media.TMDBID, 10), &detail); err != nil {
				return nil, err
			}
			value := detail.Title
			if media == "tv" {
				value = detail.Name
			}
			if value == nil || *value == "" {
				return nil, dto.Failure(dto.BadResponse)
			}
			title = text(*value)
			titles.put(key, title, now)
		}
		name := r.RequestedBy.DisplayName
		if name == "" {
			name = r.RequestedBy.Username
		}
		if name == "" {
			name = r.RequestedBy.PlexUsername
		}
		if name == "" {
			name = r.RequestedBy.Email
		}
		out.Requests = append(out.Requests, dto.MediaRequest{ID: r.ID, Title: title, MediaType: media, RequestedBy: text(name), CreatedAt: text(*r.CreatedAt)})
	}
	return out, nil
}
