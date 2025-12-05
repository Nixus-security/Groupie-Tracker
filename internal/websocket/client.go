package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"music-platform/internal/database"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	RoomCode string
	UserID   int64
	Pseudo   string
}

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, user *database.User) {
	roomCode := strings.TrimPrefix(r.URL.Path, "/ws/")
	if roomCode == "" {
		http.Error(w, "Code salle requis", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Erreur upgrade WebSocket: %v", err)
		return
	}

	client := &Client{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		RoomCode: roomCode,
		UserID:   user.ID,
		Pseudo:   user.Pseudo,
	}

	hub.Register(client)

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Erreur WebSocket: %v", err)
			}
			break
		}

		c.handleMessage(message)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(message []byte) {
	var msg Message
	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "chat":
		c.Hub.BroadcastToRoom(c.RoomCode, map[string]interface{}{
			"type":    "chat",
			"user_id": c.UserID,
			"pseudo":  c.Pseudo,
			"message": msg.Data["message"],
		})

	case "ready":
		c.Hub.BroadcastToRoom(c.RoomCode, map[string]interface{}{
			"type":    "player_ready",
			"user_id": c.UserID,
			"pseudo":  c.Pseudo,
		})

	case "start_game":
		c.Hub.BroadcastToRoom(c.RoomCode, map[string]interface{}{
			"type":    "game_starting",
			"user_id": c.UserID,
		})

	case "submit_answer":
		c.Hub.BroadcastToRoom(c.RoomCode, map[string]interface{}{
			"type":    "answer_submitted",
			"user_id": c.UserID,
			"pseudo":  c.Pseudo,
		})

	case "ping":
		c.Hub.SendToClient(c, map[string]interface{}{
			"type": "pong",
		})
	}
}

type Message struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}
