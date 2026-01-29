package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Server struct {
	rooms    *RoomRegistry
	upgrader websocket.Upgrader
}

func NewServer() *Server {
	s := &Server{
		rooms: NewRoomRegistry(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	go s.flushLoop()
	return s
}

func (s *Server) flushLoop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		now := time.Now().UnixMilli()
		rooms := s.rooms.Rooms()
		for _, r := range rooms {
			if msg := r.FlushCompletedBucket(now); msg != nil {
				r.broadcastJSON(msg)
			}
		}
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/rooms", s.listRooms)
	mux.HandleFunc("/v1/rooms/", s.handleRooms)
	return mux
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	// net/http ServeMux treats patterns ending in '/' as a subtree match.
	// We want:
	//   - GET /v1/rooms and GET /v1/rooms/ => listRooms
	//   - /v1/rooms/{roomId}/... => room handlers
	// Also treat double-slash variants (e.g. /v1/rooms//) as listRooms.
	if strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/rooms/"), "/") == "" {
		s.listRooms(w, r)
		return
	}
	// fallback to room handler
	s.handle(w, r)
}

func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	activeOnly := true
	q := strings.TrimSpace(r.URL.Query().Get("activeOnly"))
	if q != "" {
		v := strings.ToLower(q)
		if v == "false" || v == "0" {
			activeOnly = false
		}
	}

	nowUnixMs := time.Now().UnixMilli()
	rooms := s.rooms.ListRooms(nowUnixMs, activeOnly)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(RoomsListResponse{Rooms: rooms})
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	// /v1/rooms/{roomId}/events
	// /v1/rooms/{roomId}/ws
	p := strings.TrimPrefix(r.URL.Path, "/v1/rooms/")
	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	roomID := parts[0]
	endpoint := parts[1]
	if roomID == "" {
		http.NotFound(w, r)
		return
	}

	switch endpoint {
	case "events":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.postEvents(w, r, roomID)
		return
	case "ws":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getWS(w, r, roomID)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func (s *Server) postEvents(w http.ResponseWriter, r *http.Request, roomID string) {
	token := strings.TrimSpace(r.Header.Get("X-EQLog-Token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	serverRecvUnixMs := time.Now().UnixMilli()

	var req PublishBatchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	room, err := s.rooms.GetOrCreate(roomID, token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	log.Printf("room=%s publisher=%s events=%d", roomID, req.PublisherID, len(req.Events))
	updates := room.IngestBatch(serverRecvUnixMs, req)
	for _, u := range updates {
		room.broadcastJSON(u)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(OkResponse{Ok: true})
}

func (s *Server) getWS(w http.ResponseWriter, r *http.Request, roomID string) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		log.Printf("ws unauthorized: room=%s missing token", roomID)
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	room, err := s.rooms.GetOrCreate(roomID, token)
	if err != nil {
		log.Printf("ws unauthorized: room=%s err=%v", roomID, err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: room=%s err=%v", roomID, err)
		return
	}

	remote := r.RemoteAddr
	log.Printf("ws connect: room=%s remote=%s", roomID, remote)

	client := newWSClient(c)
	room.addSub(client)

	// Keepalive + close detection: read loop.
	_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error {
		_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	go s.writePump(roomID, room, client)

	// Always enqueue initial snapshot immediately.
	_ = client.enqueueJSON(room.Snapshot())

	for {
		if _, _, err := c.ReadMessage(); err != nil {
			log.Printf("ws read closed: room=%s remote=%s err=%v", roomID, remote, err)
			break
		}
	}

	room.removeSub(client)
	client.close()
	log.Printf("ws disconnect: room=%s remote=%s", roomID, remote)
}

func (s *Server) writePump(roomID string, room *Room, c *wsClient) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	defer func() {
		room.removeSub(c)
		c.close()
	}()

	for {
		select {
		case <-c.done:
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("ws write failed: room=%s err=%v", roomID, err)
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("ws ping failed: room=%s err=%v", roomID, err)
				return
			}
		}
	}
}
