package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPostEvents_TokenAuthAndOK(t *testing.T) {
	s := NewServer()
	h := s.Routes()

	body := []byte(`{"publisherId":"p1","sentAtUnixMs":1,"events":[]}`)

	// First use sets the room token.
	req := httptest.NewRequest(http.MethodPost, "/v1/rooms/r1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-EQLog-Token", "t1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	// Wrong token should be rejected.
	req2 := httptest.NewRequest(http.MethodPost, "/v1/rooms/r1/events", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-EQLog-Token", "wrong")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", w2.Code, http.StatusUnauthorized)
	}

	// Missing token should be rejected.
	req3 := httptest.NewRequest(http.MethodPost, "/v1/rooms/r2/events", bytes.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", w3.Code, http.StatusUnauthorized)
	}
}

func TestRoom_FlushCompletedBucket_Cadence(t *testing.T) {
	r := newRoom("r1", "t1")
	r.bucketSec = 5
	// Add a dummy subscriber to enable flush.
	r.addSub(&wsClient{send: make(chan []byte, 1), done: make(chan struct{})})

	// Within the first bucket: first call should flush previous bucket (start=0).
	m1 := r.FlushCompletedBucket(1_000)
	if m1 == nil {
		t.Fatalf("expected flush message")
	}
	if m1.Type != "bucket_update" {
		t.Fatalf("type=%s want=bucket_update", m1.Type)
	}
	if m1.BucketStartUnixMs != 0 {
		t.Fatalf("bucketStart=%d want=0", m1.BucketStartUnixMs)
	}

	// Same bucket should not flush again.
	if m := r.FlushCompletedBucket(2_000); m != nil {
		t.Fatalf("expected no flush within same bucket")
	}

	// Cross bucket boundary: should flush previous completed bucket (start=0).
	m2 := r.FlushCompletedBucket(6_000)
	if m2 == nil {
		t.Fatalf("expected flush message at boundary")
	}
	if m2.BucketStartUnixMs != 0 {
		t.Fatalf("bucketStart=%d want=0", m2.BucketStartUnixMs)
	}

	// Next boundary: should flush bucket start=5000.
	m3 := r.FlushCompletedBucket(11_000)
	if m3 == nil {
		t.Fatalf("expected flush message at next boundary")
	}
	if m3.BucketStartUnixMs != 5_000 {
		t.Fatalf("bucketStart=%d want=5000", m3.BucketStartUnixMs)
	}
}

func TestWS_InitialSnapshot(t *testing.T) {
	s := NewServer()
	h := s.Routes()
	server := httptest.NewServer(h)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/rooms/r1/ws?token=t1"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial err=%v", err)
	}
	defer func() { _ = c.Close() }()

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read err=%v", err)
	}

	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if header.Type != "bucket_snapshot" {
		t.Fatalf("type=%s want=bucket_snapshot", header.Type)
	}
}

func TestListRooms_TrailingSlashAndNoSlash(t *testing.T) {
	s := NewServer()
	h := s.Routes()

	assertOK := func(path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d want=%d body=%s", path, w.Code, http.StatusOK, w.Body.String())
		}
		var res RoomsListResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("path=%s invalid json: %v body=%s", path, err, w.Body.String())
		}
		if res.Rooms == nil {
			t.Fatalf("path=%s expected rooms to be present", path)
		}
	}

	assertOK("/v1/rooms")
	assertOK("/v1/rooms/")
}

func TestListRooms_IncludesActiveRoom(t *testing.T) {
	s := NewServer()
	h := s.Routes()

	body := []byte(`{"publisherId":"p1","sentAtUnixMs":1,"events":[]}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/rooms/r1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-EQLog-Token", "t1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/rooms", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", w2.Code, http.StatusOK, w2.Body.String())
	}

	var res RoomsListResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(res.Rooms) == 0 {
		t.Fatalf("expected rooms list to be non-empty")
	}

	found := false
	for _, r := range res.Rooms {
		if r.RoomID == "r1" {
			found = true
			if r.PublisherCount < 1 {
				t.Fatalf("publisherCount=%d want>=1", r.PublisherCount)
			}
		}
	}
	if !found {
		t.Fatalf("expected room r1 in response")
	}
}
