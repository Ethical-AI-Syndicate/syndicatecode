package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	"nhooyr.io/websocket"
	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	"nhooyr.io/websocket/wsjson"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestHandleEventStream_DeliversEventsAndResumes(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	now := time.Now().UTC()
	appendEvent(t, eventStore, created.ID, "turn_completed", now)
	appendEvent(t, eventStore, created.ID, "mcp.call", now.Add(10*time.Millisecond))

	server := &Server{
		sessionMgr: sessionMgr,
		eventStore: eventStore,
	}
	ts := httptest.NewServer(http.HandlerFunc(server.handleEventStream))
	defer ts.Close()

	conn := dialEventStream(t, ts, created.ID, "")
	connClosed := false
	defer func() {
		if connClosed {
			return
		}
		//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
		if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
			t.Errorf("close conn: %v", err)
		}
	}()

	firstEvent := readStreamEvent(t, conn)
	if firstEvent.EventType != "session_started" {
		t.Fatalf("expected session_started, got %s", firstEvent.EventType)
	}

	secondEvent := readStreamEvent(t, conn)
	if secondEvent.EventType != "turn_completed" {
		t.Fatalf("expected turn_completed, got %s", secondEvent.EventType)
	}

	thirdEvent := readStreamEvent(t, conn)
	if thirdEvent.EventType != "mcp.call" {
		t.Fatalf("expected mcp.call, got %s", thirdEvent.EventType)
	}

	cursorValue := fmt.Sprintf("%s|%s", thirdEvent.Timestamp.Format(time.RFC3339), thirdEvent.ID)

	appendEvent(t, eventStore, created.ID, "model_invocation", now.Add(20*time.Millisecond))

	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close conn: %v", err)
	}
	connClosed = true

	conn2 := dialEventStream(t, ts, created.ID, cursorValue)
	defer func() {
		//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
		if err := conn2.Close(websocket.StatusNormalClosure, ""); err != nil {
			t.Errorf("close conn2: %v", err)
		}
	}()

	nextEvent := readStreamEvent(t, conn2)
	if nextEvent.EventType != "model_invocation" {
		t.Fatalf("expected model_invocation after cursor, got %s", nextEvent.EventType)
	}
}

//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
func readStreamEvent(t *testing.T, conn *websocket.Conn) audit.Event {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ev audit.Event
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatalf("failed to read websocket event: %v", err)
	}
	return ev
}

//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
func dialEventStream(t *testing.T, ts *httptest.Server, sessionID, cursor string) *websocket.Conn {
	params := url.Values{}
	params.Set("session_id", sessionID)
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	wsURL := fmt.Sprintf("ws%s/api/v1/events/stream?%s", strings.TrimPrefix(ts.URL, "http"), params.Encode())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	return conn
}

func appendEvent(t *testing.T, store *audit.EventStore, sessionID, eventType string, at time.Time) {
	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Timestamp: at,
		EventType: eventType,
		Actor:     "system",
		Payload:   json.RawMessage(`{"event":"` + eventType + `"}`),
	}); err != nil {
		t.Fatalf("failed to append event %s: %v", eventType, err)
	}
}
