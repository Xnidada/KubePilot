package workload

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	maxTerminalSessions       = 100
	maxTerminalSessionsPerUser = 3
	terminalSessionDuration    = 30 * time.Minute
	terminalIdleTimeout        = 15 * time.Minute
	terminalWriteTimeout       = 10 * time.Second
	terminalPingInterval       = 30 * time.Second
	terminalReadLimit          = 64 * 1024
)

type terminalSessionRegistry struct {
	mu      sync.Mutex
	total   int
	byUser  map[uint]int
}

var terminalSessions = terminalSessionRegistry{
	byUser: make(map[uint]int),
}

func beginTerminalSession(c *gin.Context) (func(), bool) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint)
	if !exists || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return nil, false
	}
	if !terminalSessions.acquire(userID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "terminal session limit reached"})
		return nil, false
	}
	return func() { terminalSessions.release(userID) }, true
}

func (r *terminalSessionRegistry) acquire(userID uint) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.total >= maxTerminalSessions || r.byUser[userID] >= maxTerminalSessionsPerUser {
		return false
	}
	r.total++
	r.byUser[userID]++
	return true
}

func (r *terminalSessionRegistry) release(userID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byUser[userID] > 1 {
		r.byUser[userID]--
	} else {
		delete(r.byUser, userID)
	}
	if r.total > 0 {
		r.total--
	}
}

type websocketMessage struct {
	messageType int
	data        []byte
}

type websocketSession struct {
	conn       *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	send       chan websocketMessage
	writerDone chan struct{}
	closeOnce  sync.Once
}

func newWebsocketSession(conn *websocket.Conn) *websocketSession {
	ctx, cancel := context.WithTimeout(context.Background(), terminalSessionDuration)
	session := &websocketSession{
		conn:       conn,
		ctx:        ctx,
		cancel:     cancel,
		send:       make(chan websocketMessage, 64),
		writerDone: make(chan struct{}),
	}

	conn.SetReadLimit(terminalReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(terminalIdleTimeout))
	conn.SetPongHandler(func(string) error { return nil })
	go session.writePump()
	return session
}

func (s *websocketSession) Context() context.Context {
	return s.ctx
}

func (s *websocketSession) ReadMessage() (int, []byte, error) {
	if err := s.conn.SetReadDeadline(time.Now().Add(terminalIdleTimeout)); err != nil {
		return 0, nil, err
	}
	return s.conn.ReadMessage()
}

func (s *websocketSession) SendText(message string) bool {
	return s.Send(websocket.TextMessage, []byte(message))
}

func (s *websocketSession) Send(messageType int, data []byte) bool {
	message := websocketMessage{
		messageType: messageType,
		data:        append([]byte(nil), data...),
	}
	select {
	case <-s.ctx.Done():
		return false
	default:
	}
	select {
	case s.send <- message:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *websocketSession) writePump() {
	defer close(s.writerDone)
	ticker := time.NewTicker(terminalPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		select {
		case message := <-s.send:
			_ = s.conn.SetWriteDeadline(time.Now().Add(terminalWriteTimeout))
			if err := s.conn.WriteMessage(message.messageType, message.data); err != nil {
				s.cancel()
				return
			}
		case <-ticker.C:
			deadline := time.Now().Add(terminalWriteTimeout)
			if err := s.conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				s.cancel()
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *websocketSession) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.conn.Close()
		<-s.writerDone
	})
}
