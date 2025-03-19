package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nathfavour/robotics-core1/go-layer/internal/messaging"
	"github.com/sirupsen/logrus"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

// WSClient handles a WebSocket connection for real-time communication
type WSClient struct {
	conn          *websocket.Conn
	messageBroker *messaging.Broker
	send          chan []byte
	subscriptions []string
	mu            sync.Mutex
	logger        *logrus.Entry
	clientID      string
}

// NewWSClient creates a new WebSocket client
func NewWSClient(conn *websocket.Conn, messageBroker *messaging.Broker) *WSClient {
	clientID := generateClientID()
	return &WSClient{
		conn:          conn,
		messageBroker: messageBroker,
		send:          make(chan []byte, 256),
		subscriptions: make([]string, 0),
		clientID:      clientID,
		logger:        logrus.WithField("component", "ws-client").WithField("client_id", clientID),
	}
}

// Handle processes the WebSocket connection
func (c *WSClient) Handle() {
	// Start goroutines for reading and writing
	go c.writePump()
	go c.readPump()
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WSClient) readPump() {
	defer func() {
		c.unsubscribeAll()
		c.conn.Close()
		close(c.send)
		c.logger.Info("WebSocket connection closed")
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
				c.logger.WithError(err).Error("WebSocket read error")
			}
			break
		}

		// Process incoming message
		c.handleMessage(message)
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WSClient) writePump() {
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
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
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

// handleMessage processes incoming WebSocket messages
func (c *WSClient) handleMessage(message []byte) {
	var msg struct {
		Type    string          `json:"type"`
		Topic   string          `json:"topic,omitempty"`
		Payload json.RawMessage `json:"payload,omitempty"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.logger.WithError(err).Error("Failed to parse WebSocket message")
		c.sendError("invalid_message", "Failed to parse message")
		return
	}

	switch msg.Type {
	case "subscribe":
		c.handleSubscribe(msg.Topic)
	case "unsubscribe":
		c.handleUnsubscribe(msg.Topic)
	case "publish":
		c.handlePublish(msg.Topic, msg.Payload)
	default:
		c.logger.WithField("type", msg.Type).Warn("Unknown message type")
		c.sendError("unknown_type", "Unknown message type")
	}
}

func (c *WSClient) handleSubscribe(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already subscribed
	for _, t := range c.subscriptions {
		if t == topic {
			return
		}
	}

	// Subscribe to the topic
	subID, err := c.messageBroker.Subscribe(topic, func(data []byte) {
		select {
		case c.send <- createMessage("message", topic, data):
		default:
			c.logger.Warn("WebSocket send buffer full")
		}
	})

	if err != nil {
		c.logger.WithError(err).WithField("topic", topic).Error("Failed to subscribe")
		c.sendError("subscription_failed", "Failed to subscribe to topic")
		return
	}

	// Store subscription
	c.subscriptions = append(c.subscriptions, topic)
	c.logger.WithField("topic", topic).Info("Subscribed to topic")

	// Confirm subscription
	c.send <- createMessage("subscribed", topic, nil)
}

func (c *WSClient) handleUnsubscribe(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find and remove subscription
	for i, t := range c.subscriptions {
		if t == topic {
			if err := c.messageBroker.Unsubscribe(topic, c.clientID); err != nil {
				c.logger.WithError(err).WithField("topic", topic).Error("Failed to unsubscribe")
			}

			// Remove from subscriptions list
			c.subscriptions = append(c.subscriptions[:i], c.subscriptions[i+1:]...)

			// Confirm unsubscription
			c.send <- createMessage("unsubscribed", topic, nil)
			c.logger.WithField("topic", topic).Info("Unsubscribed from topic")
			return
		}
	}
}

func (c *WSClient) handlePublish(topic string, payload json.RawMessage) {
	if err := c.messageBroker.Publish(topic, payload); err != nil {
		c.logger.WithError(err).WithField("topic", topic).Error("Failed to publish message")
		c.sendError("publish_failed", "Failed to publish message")
		return
	}
	c.logger.WithField("topic", topic).Debug("Published message")
}

func (c *WSClient) unsubscribeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, topic := range c.subscriptions {
		if err := c.messageBroker.Unsubscribe(topic, c.clientID); err != nil {
			c.logger.WithError(err).WithField("topic", topic).Error("Failed to unsubscribe")
		}
	}
	c.subscriptions = nil
}

func (c *WSClient) sendError(code string, message string) {
	payload := map[string]string{
		"code":    code,
		"message": message,
	}
	data, _ := json.Marshal(payload)
	c.send <- createMessage("error", "", data)
}

func createMessage(msgType string, topic string, payload []byte) []byte {
	msg := map[string]interface{}{
		"type": msgType,
	}
	if topic != "" {
		msg["topic"] = topic
	}
	if payload != nil {
		var data interface{}
		if err := json.Unmarshal(payload, &data); err == nil {
			msg["payload"] = data
		} else {
			msg["payload"] = string(payload)
		}
	}
	data, _ := json.Marshal(msg)
	return data
}

func generateClientID() string {
	return fmt.Sprintf("ws-%d", time.Now().UnixNano())
}
