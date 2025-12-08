// Package websocket - client.go
// Gère une connexion WebSocket individuelle
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
	// Temps d'attente pour écrire un message
	writeWait = 10 * time.Second

	// Temps d'attente pour lire un message pong
	pongWait = 60 * time.Second

	// Intervalle d'envoi des pings
	pingPeriod = (pongWait * 9) / 10

	// Taille maximale d'un message
	maxMessageSize = 4096
)

// Client représente une connexion WebSocket
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	
	UserID   int64
	Pseudo   string
	RoomCode string
	
	// Handler pour traiter les messages reçus
	messageHandler MessageHandler
	
	closed bool
	mutex  sync.Mutex
}

// MessageHandler fonction de callback pour traiter les messages
type MessageHandler func(client *Client, msg *models.WSMessage)

// NewClient crée un nouveau client WebSocket
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

// Start démarre les goroutines de lecture et écriture
func (c *Client) Start() {
	go c.writePump()
	go c.readPump()
}

// readPump lit les messages entrants du WebSocket
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

		// Parser le message
		var wsMsg models.WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("❌ Erreur parsing message: %v", err)
			c.SendError("Message invalide")
			continue
		}

		// Traiter le message ping
		if wsMsg.Type == models.WSTypePing {
			c.Send(&models.WSMessage{Type: models.WSTypePong})
			continue
		}

		// Déléguer au handler
		if c.messageHandler != nil {
			c.messageHandler(c, &wsMsg)
		}
	}
}

// writePump envoie les messages au client WebSocket
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
				// Le hub a fermé le canal
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Envoyer les messages en attente
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
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

// Send envoie un message au client
func (c *Client) Send(msg *models.WSMessage) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case c.send <- data:
	default:
		// Buffer plein
	}
}

// SendError envoie un message d'erreur au client
func (c *Client) SendError(errMsg string) {
	c.Send(&models.WSMessage{
		Type:  models.WSTypeError,
		Error: errMsg,
	})
}

// Close ferme la connexion du client
func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.send)
}

// IsClosed vérifie si le client est fermé
func (c *Client) IsClosed() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.closed
}