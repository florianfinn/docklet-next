// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original synthetic upstream fixtures following public producer documentation.

package widget

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/stretchr/testify/require"
)

// Plex: https://developer.plex.tv/pms/ (Status / List Sessions).
func TestSW1PlexProjectionAndShape(t *testing.T) {
	long := strings.Repeat("x", 220)
	for _, test := range []struct {
		name, body string
		bad        bool
	}{
		{"empty", `{"MediaContainer":{"size":0}}`, false},
		{"session", plexBody(long), false},
		{"missing-container", `{}`, true},
		{"missing-size", `{"MediaContainer":{}}`, true},
		{"missing-metadata", `{"MediaContainer":{"size":1}}`, true},
		{"missing-user", `{"MediaContainer":{"size":1,"Metadata":[{"title":"Title","Player":{}}]}}`, true},
		{"html", `<html>upstream unavailable</html>`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/status/sessions", r.URL.Path)
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				require.Equal(t, "test-token", r.Header.Get("X-Plex-Token"))
				respond(w, test.body)
			}))
			defer server.Close()
			f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "test-token")
			defer f.close()
			out, err := fetchPlex(t.Context(), f)
			if test.bad {
				require.Equal(t, dto.BadResponse, problem(err).Code)
				return
			}
			require.NoError(t, err)
			if len(out.Sessions) > 0 {
				require.Len(t, []rune(out.Sessions[0].Title), 200)
				require.Equal(t, .5, out.Sessions[0].Progress)
			}
		})
	}
	items := make([]map[string]any, 25)
	for i := range items {
		items[i] = map[string]any{"title": "Episode", "type": "episode", "grandparentTitle": "Show", "parentIndex": 2, "index": 3, "User": map[string]string{"title": "Viewer"}, "Player": map[string]string{"title": "TV"}, "Media": []any{map[string]any{"videoResolution": "1080"}}, "TranscodeSession": map[string]any{"videoDecision": "transcode", "height": 720}}
	}
	payload, _ := json.Marshal(map[string]any{"MediaContainer": map[string]any{"size": 25, "Metadata": items}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respond(w, string(payload)) }))
	defer server.Close()
	f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "")
	defer f.close()
	out, err := fetchPlex(t.Context(), f)
	require.NoError(t, err)
	require.Len(t, out.Sessions, 20)
	require.Equal(t, "Show – Episode", out.Sessions[0].Title)
	require.Equal(t, "S2 E3", out.Sessions[0].Subtitle)
	require.Equal(t, "1080 → 720", out.Sessions[0].Quality)
	require.Equal(t, 20, out.Transcodes)
}

func overseerrList(media string, id int64) string {
	return fmt.Sprintf(`{"results":[{"id":1,"type":%q,"createdAt":"2026-11-03T12:00:00Z","requestedBy":{"displayName":"Viewer"},"media":{"tmdbId":%d,"mediaType":%q}}]}`, media, id, media)
}

// Overseerr: https://api-docs.overseerr.dev/ and the upstream overseerr-api.yml.
func TestSW1OverseerrProjectionAndSW5TitleKeys(t *testing.T) {
	var media atomic.Value
	media.Store("movie")
	var seriesTitle atomic.Value
	seriesTitle.Store("Series")
	var titleCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.Header.Get("X-Api-Key"))
		switch r.URL.Path {
		case "/api/v1/request/count":
			respond(w, `{"pending":9000000,"approved":2,"available":3}`)
		case "/api/v1/request":
			require.Equal(t, "take=5&filter=pending&sort=added", r.URL.RawQuery)
			respond(w, overseerrList(media.Load().(string), 1))
		case "/api/v1/movie/1":
			titleCalls.Add(1)
			respond(w, `{"title":"Movie"}`)
		case "/api/v1/tv/1":
			titleCalls.Add(1)
			respond(w, fmt.Sprintf(`{"name":%q}`, seriesTitle.Load().(string)))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	f := newFetchClient(dto.Config{Kind: dto.Overseerr, Address: server.URL}, "test-key")
	defer f.close()
	titles := newTitles()
	now := time.Now()
	get := func(rev int64, gen uint64) *dto.OverseerrData {
		out, err := fetchOverseerr(t.Context(), f, rev, gen, titles, now)
		require.NoError(t, err)
		return out
	}
	a := get(1, 1)
	require.Equal(t, 1_000_000, a.Pending)
	require.Equal(t, "Movie", a.Requests[0].Title)
	require.Equal(t, "Viewer", a.Requests[0].RequestedBy)
	_ = get(1, 1)
	require.EqualValues(t, 1, titleCalls.Load())
	media.Store("tv")
	b := get(1, 1)
	require.Equal(t, "Series", b.Requests[0].Title, "same TMDB ID has a distinct movie/TV namespace")
	seriesTitle.Store("New revision series")
	require.Equal(t, "New revision series", get(2, 1).Requests[0].Title)
	require.EqualValues(t, 3, titleCalls.Load(), "revision partitions title sources")
	seriesTitle.Store("Post-action series")
	require.Equal(t, "Post-action series", get(2, 2).Requests[0].Title)
	require.EqualValues(t, 4, titleCalls.Load(), "generation partitions derived results")
	now = now.Add(24 * time.Hour)
	_ = get(2, 2)
	require.EqualValues(t, 5, titleCalls.Load(), "24-hour title TTL")
}

func TestSW1OverseerrRejectsMissingRequiredFields(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":[{}]}`, strings.Replace(overseerrList("movie", 1), `"createdAt":"2026-11-03T12:00:00Z",`, "", 1), overseerrList("other", 1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/count") {
				respond(w, `{"pending":0,"approved":0,"available":0}`)
			} else {
				respond(w, body)
			}
		}))
		f := newFetchClient(dto.Config{Kind: dto.Overseerr, Address: server.URL}, "")
		_, err := fetchOverseerr(t.Context(), f, 1, 1, newTitles(), time.Now())
		require.Equal(t, dto.BadResponse, problem(err).Code)
		f.close()
		server.Close()
	}
}

func minecraftServer(t *testing.T, payload []byte, oversized bool) string {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(conn)
		length, err := readVarInt(r)
		if err != nil {
			return
		}
		b := make([]byte, length)
		if _, err = io.ReadFull(r, b); err != nil {
			return
		}
		frame := bytes.NewReader(b)
		id, _ := readVarInt(frame)
		protocol, _ := readVarInt(frame)
		hostLen, _ := readVarInt(frame)
		host := make([]byte, hostLen)
		_, _ = io.ReadFull(frame, host)
		var port uint16
		_ = binary.Read(frame, binary.BigEndian, &port)
		next, _ := readVarInt(frame)
		if id != 0 || protocol != mathProtocolVersion || len(host) == 0 || port == 0 || next != 1 {
			t.Error("invalid Minecraft status handshake")
			return
		}
		n, _ := readVarInt(r)
		status := make([]byte, n)
		_, _ = io.ReadFull(r, status)
		if !bytes.Equal(status, []byte{0}) {
			t.Error("invalid status request")
			return
		}
		if oversized {
			_ = varInt(conn, minecraftLimit+1)
			return
		}
		var response bytes.Buffer
		_ = varInt(&response, 0)
		_ = varInt(&response, len(payload))
		response.Write(payload)
		_ = packet(conn, response.Bytes())
	}()
	return listener.Addr().String()
}

func TestSW1MinecraftRealStatusProtocol(t *testing.T) {
	players := make([]map[string]string, 20)
	for i := range players {
		players[i] = map[string]string{"name": fmt.Sprintf("Player%d", i)}
	}
	body, _ := json.Marshal(map[string]any{"version": map[string]any{"name": "Java 1.21", "protocol": 767}, "players": map[string]any{"online": 20, "max": 30, "sample": players}, "description": map[string]any{"text": "§aWelcome ", "extra": []any{map[string]any{"text": "players"}}}, "favicon": "ignored"})
	ctx, cancel := context.WithTimeout(t.Context(), fetchBudget)
	defer cancel()
	result, err := fetchMinecraft(ctx, minecraftServer(t, body, false))
	require.NoError(t, err)
	require.Equal(t, "Welcome players", result.MOTD)
	require.Equal(t, 20, result.Online)
	require.Equal(t, 30, result.Max)
	require.Len(t, result.Players, 12)
	require.Equal(t, "Java 1.21", result.Version)
	for _, bad := range [][]byte{[]byte(`{}`), []byte(`<html>`), []byte(`{"version":{"name":"Java"},"players":{"online":1},"description":"hello"}`)} {
		_, err = fetchMinecraft(ctx, minecraftServer(t, bad, false))
		require.Equal(t, dto.BadResponse, problem(err).Code)
	}
	_, err = fetchMinecraft(ctx, minecraftServer(t, nil, true))
	require.Equal(t, dto.BadResponse, problem(err).Code)
	_, err = readVarInt(bytes.NewReader([]byte{255, 255, 255, 255, 255, 0}))
	require.Error(t, err)
	motd, ok := flatten([]any{"§aHi", map[string]any{"text": " there"}}, 0)
	require.True(t, ok)
	require.Equal(t, "Hi there", stripFormatting(motd))
}

// AUTO-2: a requester known only by email is shown without a name; the address never leaves the hub.
func TestOverseerrRequesterEmailStaysOnHub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request/count":
			respond(w, `{"pending":1,"approved":0,"available":0}`)
		case "/api/v1/request":
			respond(w, `{"results":[{"id":1,"type":"movie","createdAt":"2026-11-03T12:00:00Z","requestedBy":{"email":"viewer@example.com"},"media":{"tmdbId":1,"mediaType":"movie"}}]}`)
		case "/api/v1/movie/1":
			respond(w, `{"title":"Movie"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	f := newFetchClient(dto.Config{Kind: dto.Overseerr, Address: server.URL}, "test-key")
	defer f.close()
	out, err := fetchOverseerr(t.Context(), f, 1, 1, newTitles(), time.Now())
	require.NoError(t, err)
	require.Equal(t, "", out.Requests[0].RequestedBy)
	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "example.com")
}
