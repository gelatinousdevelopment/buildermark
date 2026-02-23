package handler

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const wsMagicKey = "258EAFA5-E914-47DA-95CA-5AB9AA286923"

// wsConn is a minimal WebSocket connection wrapping a hijacked net.Conn.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex // serialises writes
}

// upgradeWebSocket performs the HTTP→WebSocket upgrade handshake.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, fmt.Errorf("not a websocket upgrade request")
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, fmt.Errorf("missing Connection: Upgrade header")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key header")
	}

	h := sha1.New()
	h.Write([]byte(key + wsMagicKey))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response writer does not support hijacking")
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack: %w", err)
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}

	return &wsConn{conn: conn, br: buf.Reader}, nil
}

// writeText sends a text frame to the client.
func (c *wsConn) writeText(data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	// FIN=1, opcode=text(0x1)
	var frame []byte
	frame = append(frame, 0x81)
	switch {
	case len(data) < 126:
		frame = append(frame, byte(len(data)))
	case len(data) < 65536:
		frame = append(frame, 126)
		frame = append(frame, byte(len(data)>>8), byte(len(data)))
	default:
		frame = append(frame, 127)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(len(data)))
		frame = append(frame, b...)
	}
	frame = append(frame, data...)
	_, err := c.conn.Write(frame)
	return err
}

// writeJSON marshals v as JSON and sends it as a text frame.
func (c *wsConn) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeText(data)
}

// readLoop reads and discards incoming frames, responding to pings and
// detecting close frames. It blocks until the connection is closed or errors.
// The caller is responsible for closing the connection afterward.
func (c *wsConn) readLoop() {
	for {
		if err := c.readFrame(); err != nil {
			return
		}
	}
}

func (c *wsConn) readFrame() error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.br, header); err != nil {
		return err
	}

	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(c.br, ext); err != nil {
			return err
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(c.br, ext); err != nil {
			return err
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}

	switch opcode {
	case 0x8: // close
		return io.EOF
	case 0x9: // ping → pong
		c.writePong(payload)
	}
	return nil
}

func (c *wsConn) writePong(payload []byte) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	// FIN=1, opcode=pong(0xA)
	frame := []byte{0x8A, byte(len(payload))}
	frame = append(frame, payload...)
	c.conn.Write(frame)
}

func (c *wsConn) close() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.conn.Write([]byte{0x88, 0x00}) // close frame
	return c.conn.Close()
}

// wsMessage is the top-level JSON envelope sent over WebSocket.
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// wsClient represents a single connected WebSocket client.
type wsClient struct {
	ws   *wsConn
	send chan []byte
}

// wsHub manages connected WebSocket clients and broadcasts messages.
type wsHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

func newWSHub() *wsHub {
	return &wsHub{clients: make(map[*wsClient]struct{})}
}

func (h *wsHub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *wsHub) unregister(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *wsHub) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			// client too slow, drop message
		}
	}
}

func (h *wsHub) broadcastEvent(eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg, err := json.Marshal(wsMessage{Type: eventType, Data: raw})
	if err != nil {
		return
	}
	h.broadcast(msg)
}

func (h *wsHub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// handleWS upgrades the HTTP connection to WebSocket and manages the client lifecycle.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgradeWebSocket(w, r)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}

	client := &wsClient{
		ws:   ws,
		send: make(chan []byte, 64),
	}
	s.ws.register(client)

	// Writer goroutine: sends queued messages to the client.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer ws.close()

		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					return
				}
				if err := ws.writeText(msg); err != nil {
					return
				}
			case <-ticker.C:
				// Send ping to keep connection alive.
				ws.wmu.Lock()
				_, err := ws.conn.Write([]byte{0x89, 0x00}) // ping frame
				ws.wmu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	// Reader loop blocks until disconnect.
	ws.readLoop()

	s.ws.unregister(client)
	close(client.send)
}
