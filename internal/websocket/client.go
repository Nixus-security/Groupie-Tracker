package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"groupie-tracker/internal/models"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte

	UserID   int64
	Pseudo   string
	RoomCode string

	messageHandler MessageHandler

	closed bool
	mutex  sync.Mutex
}

type MessageHandler func(client *Client, msg *models.WSMessage)

func NewClient(hub *Hub, conn *websocket.Conn, userID int64, pseudo, roomCode string, handler MessageHandler) *Client {
	return &Client{
		hub:            hub,
		conn:           conn,
		send:           make(chan []byte, 256),
		UserID:         userID,
		Pseudo:         pseudo,
		RoomCode:       roomCode,
		messageHandler: handler,
	}
}

func (c *Client) Start() {
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ Erreur WebSocket: %v", err)
			}
			break
		}

		var wsMsg models.WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("❌ Erreur parsing message: %v - Data: %s", err, string(message))
			c.SendError("Message invalide")
			continue
		}

		log.Printf("[WS] 📨 Client %d (%s) -> type=%s", c.UserID, c.Pseudo, wsMsg.Type)

		if wsMsg.Type == models.WSTypePing {
			c.Send(&models.WSMessage{Type: models.WSTypePong})
			continue
		}

		if c.messageHandler != nil {
			c.messageHandler(c, &wsMsg)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("❌ Erreur écriture WebSocket: %v", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) Send(msg *models.WSMessage) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("❌ Erreur marshal message: %v", err)
		return
	}

	log.Printf("[WS] 📤 -> Client %d (%s): type=%s", c.UserID, c.Pseudo, msg.Type)

	select {
	case c.send <- data:
	default:
		log.Printf("⚠️ Buffer plein pour client %d", c.UserID)
	}
}

func (c *Client) SendError(errMsg string) {
	c.Send(&models.WSMessage{
		Type:  models.WSTypeError,
		Error: errMsg,
	})
}

func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.send)
}

func (c *Client) IsClosed() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closed
}