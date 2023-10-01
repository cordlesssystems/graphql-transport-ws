package internal_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cordlesssystems/graphql-transport-ws/internal"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler_ServeHTTP(t *testing.T) {

	t.Run("complete_server_sent", func(t *testing.T) {

		conn, cleanup := getConn(t, subOneMessage)
		defer cleanup()

		connInit(t, conn)
		subscribe(t, conn, "123", "query")

		testNextMessage(t, conn, "connection_ack")
		testNextMessage(t, conn, "next")
		testNextMessage(t, conn, "complete")
		testNoMoreMessages(t, conn)
	})
	t.Run("complete_client_sent", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessageAndWaitCancel)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		subscribe(t, conn, "123", "query")
		testNextMessage(t, conn, "next")
		testNoMoreMessages(t, conn)
		complete(t, conn, "123")
		testNoMoreMessages(t, conn)
	})
	t.Run("init_subscribe_first", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessage)
		defer cleanup()

		subscribe(t, conn, "123", "query")
		testCloseReason(t, conn, 4403, "Forbidden")
	})
	t.Run("init_timeout", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessage, internal.WithConnectionInitTimeout(time.Millisecond*10))
		defer cleanup()
		time.Sleep(time.Millisecond * 20)
		testCloseReason(t, conn, 4408, "Connection initialisation timeout")
	})
	t.Run("init_too_many", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessage, internal.WithConnectionInitTimeout(time.Millisecond*10))
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		connInit(t, conn)
		testCloseReason(t, conn, 4429, "Too many initialisation requests")
	})
	t.Run("error_in_subscription", func(t *testing.T) {
		conn, cleanup := getConn(t, subErr)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		subscribe(t, conn, "123", "query")
		testNextMessage(t, conn, "error")
		testNoMoreMessages(t, conn)
	})
	t.Run("error_in_subscription_in_progress", func(t *testing.T) {
		conn, cleanup := getConn(t, subErrInProgress)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		subscribe(t, conn, "123", "query")
		testNextMessage(t, conn, "error")
		testNoMoreMessages(t, conn)
	})
	t.Run("unknown_message_type", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessage)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		sendUnknownMessageType(t, conn)
		testCloseReason(t, conn, 4400, "Unknown message type")
	})
	t.Run("wrong_json", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessage)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		sendWrongJson(t, conn)
		testCloseReason(t, conn, 4400, "Message parsing error")
	})
	t.Run("close_client_sent", func(t *testing.T) {
		conn, cleanup := getConn(t, subOneMessageAndWaitCancel)
		defer cleanup()

		connInit(t, conn)
		testNextMessage(t, conn, "connection_ack")
		subscribe(t, conn, "123", "query")
		testNextMessage(t, conn, "next")
		testNoMoreMessages(t, conn)
		err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Fatalf("unable to send close msg: %v", err)
		}
		testNoMoreMessages(t, conn)
	})

}

// Helpers

type msgT struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		Query string
	} `json:"payload"`
}

type timeoutI interface {
	Timeout() bool
}

func getConn(
	t *testing.T,
	subscription internal.HandleSubscribeFunc,
	opts ...func(h *internal.Handler),
) (*websocket.Conn, func()) {
	h, err := internal.NewHandler(
		subscription,
		opts...,
	)

	if err != nil {
		t.Fatalf("unable to initialise handler: %v", err)
	}

	server := httptest.NewServer(h)

	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", -1), http.Header{})
	if err != nil {
		server.Close()
		t.Fatalf("unable to dial: %v", err)
	}
	return conn, func() {
		conn.Close()
		server.Close()
	}
}

func subscribe(t *testing.T, conn *websocket.Conn, id string, query string) {

	msg := msgT{
		Type: "subscribe",
		ID:   id,
		Payload: struct{ Query string }{
			Query: query,
		},
	}

	err := conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set write deadline: %v", err)
	}

	err = conn.WriteJSON(msg)
	if err != nil {
		t.Fatalf("unable to send subscribe msg: %v", err)
	}
}

func complete(t *testing.T, conn *websocket.Conn, id string) {

	msg := msgT{
		Type: "complete",
		ID:   id,
	}

	err := conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set write deadline: %v", err)
	}

	err = conn.WriteJSON(msg)
	if err != nil {
		t.Fatalf("unable to send subscribe msg: %v", err)
	}
}

func connInit(t *testing.T, conn *websocket.Conn) {

	msg := msgT{
		Type: "connection_init",
	}

	err := conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set write deadline: %v", err)
	}

	err = conn.WriteJSON(msg)
	if err != nil {
		t.Fatalf("unable to send subscribe msg: %v", err)
	}
}

func sendUnknownMessageType(t *testing.T, conn *websocket.Conn) {

	msg := msgT{
		Type: "unknown",
	}

	err := conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set write deadline: %v", err)
	}

	err = conn.WriteJSON(msg)
	if err != nil {
		t.Fatalf("unable to send subscribe msg: %v", err)
	}
}

func sendWrongJson(t *testing.T, conn *websocket.Conn) {

	msg := []byte(`{"type": "unknown",}`)

	err := conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set write deadline: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		t.Fatalf("unable to send subscribe msg: %v", err)
	}
}

func testNoMoreMessages(t *testing.T, conn *websocket.Conn) {

	err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	if err != nil {
		t.Fatalf("unable to set read deadline: %v", err)
	}

	_, m, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("found not expected messages from server: %s", m)
	}

	if timeoutErr, ok := err.(timeoutI); ok {
		if !timeoutErr.Timeout() {
			t.Fatalf("expected timeout err, got: %v", err)
		}
		return // ok
	}

	t.Fatalf("not expected error while reading the socket: %v", err)
}

func testNextMessage(t *testing.T, conn *websocket.Conn, expectedType string) {

	err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
	if err != nil {
		t.Fatalf("unable to set read deadline: %v", err)
	}

	msg := msgT{}
	err = conn.ReadJSON(&msg)
	if err != nil {
		t.Errorf("unable to parse json: %v", err)
		return
	}

	if msg.Type != expectedType {
		t.Errorf("received not expected message type: %s (instead of %s)", msg.Type, expectedType)
	}
}

func testCloseReason(t *testing.T, conn *websocket.Conn, code int, message string) {
	_, _, err := conn.ReadMessage()

	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}

	if closeErr.Code != code {
		t.Errorf("expected close code %d, got %d", code, closeErr.Code)
	}
	if closeErr.Text != message {
		t.Errorf("expected close message %q, got %q", message, closeErr.Text)
	}
}

func subOneMessage(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {
	result := make(chan internal.ExecutionResult)
	err := make(chan error)
	go func() {
		defer close(result)
		defer close(err)
		result <- []byte(`{"data": 42}`)
	}()
	return result, err, nil
}

func subOneMessageAndWaitCancel(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {
	result := make(chan internal.ExecutionResult)
	err := make(chan error)
	go func() {
		defer close(result)
		defer close(err)
		result <- []byte(`{"data": 42}`)
		<-ctx.Done() // wait forever until cancellation
	}()
	return result, err, nil
}

func subErr(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {
	return nil, nil, errors.New("test error")
}

func subErrInProgress(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {
	result := make(chan internal.ExecutionResult)
	err := make(chan error)
	go func() {
		defer close(result)
		defer close(err)
		err <- errors.New("test error")
	}()
	return result, err, nil
}

func infiniteSubscription(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {
	result := make(chan internal.ExecutionResult)
	err := make(chan error)
	go func() {
		defer close(result)
		defer close(err)
		for {
			select {
			case <-ctx.Done():
				return
			case result <- []byte(`{"data": {"infinite": true}}`):
				time.Sleep(time.Millisecond * 50)
			}
		}
	}()
	return result, err, nil
}
