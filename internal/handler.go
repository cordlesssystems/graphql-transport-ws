package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	errInitTimeout = errors.New("initialisation timeout")
)

type (
	executionResult json.RawMessage
)

type (
	handleInitFunc      = func(ctx context.Context, payload genericPayload) (context.Context, genericPayload, error)
	handleSubscribeFunc = func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan executionResult, <-chan error, error)
)

type Handler struct {
	Upgrader                  websocket.Upgrader
	connectionInitWaitTimeout time.Duration
	handleInit                handleInitFunc
	handleSubscribe           handleSubscribeFunc
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	conn, err := h.Upgrader.Upgrade(w, r, http.Header{})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(`unable to upgrade`))
		return
	}
	defer conn.Close()

	var newCtx context.Context
	newCtx, err = connectionInit(ctx, conn, h.connectionInitWaitTimeout, h.handleInit)
	if err != nil {
		return
	}

	err = listen(newCtx, conn, h.handleSubscribe)
	if err != nil {
		return
	}

}

// connectionInit makes handshake
func connectionInit(ctx context.Context, conn *websocket.Conn, timeout time.Duration, handleInit handleInitFunc) (context.Context, error) {
	var (
		errCh = make(chan error)
		msgCh = make(chan message)
		err   error
	)

	go func() {
		defer close(errCh)
		defer close(msgCh)

		var (
			msg message
		)
		err = conn.ReadJSON(&msg)
		if err != nil {
			errCh <- err
			return
		}
		msgCh <- msg
	}()

	var (
		msg message
	)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case msg = <-msgCh:
		break
	case err = <-errCh:

	case <-timer.C:
		err = conn.WriteMessage(websocket.CloseMessage, closeInitTimeout)
		if err != nil {
			return ctx, fmt.Errorf(
				"unable to write CloseMessage control frame after initialisation timeout: %w: %w",
				errInitTimeout,
				err,
			)
		}
		return ctx, errInitTimeout
	}

	return ctx, nil

}

func listen(ctx context.Context, conn *websocket.Conn, handleSubscribe handleSubscribeFunc) error {
	var (
		reader   io.Reader
		err      error
		closeErr *websocket.CloseError
		msg      message
		opMu     sync.Mutex
		op       = make(map[string]func())
		cancel   func()
		ok       bool
		newCtx   context.Context
	)

	var wg sync.WaitGroup
	for {
		_, reader, err = conn.NextReader()
		if errors.As(err, &closeErr) {
			return nil
		}
		if err != nil {
			return err
		}

		err = json.NewDecoder(reader).Decode(&msg)
		if err != nil {
			// write close message with relevant code
		}

		writeCh := make(chan message, 100)
		go runWriter(conn, writeCh)

		switch msg.Type {
		case msgPing:
			// implement me
		case msgPong:
			// implement me
		case msgComplete:
			opMu.Lock()
			cancel, ok = op[msg.ID]
			if ok {
				cancel()
			}
			opMu.Unlock()
		case msgSubscribe:

			newCtx, cancel = context.WithCancel(ctx)
			opMu.Lock()
			op[msg.ID] = cancel
			opMu.Unlock()

			wg.Add(1)
			go func() {
				runSubscription(newCtx, msg.ID, msg.Payload, handleSubscribe, writeCh)

				wg.Done()
				opMu.Lock()
				delete(op, msg.ID)
				opMu.Unlock()
			}()
		default:
			// write close message with relevant code
		}

	}
}

func runWriter(conn *websocket.Conn, ch <-chan message) {

}

func runSubscription(
	ctx context.Context,
	id string,
	payload json.RawMessage,
	handleSubscribe handleSubscribeFunc,
	writeCh chan<- message,
) {

	var (
		exRes    executionResult
		ok       bool
		fatalErr = graphqlError{
			Message: "error in handleSubscription",
		}
	)

	execResultCh, errCh, err := handleSubscribe(ctx, id, payload)

	if err != nil {

		b, serErr := json.Marshal(fatalErr)
		if serErr != nil {
			panic(serErr)
		}

		writeCh <- message{
			Payload: b,
			ID:      id,
			Type:    msgError,
		}
		return
	}

	for {
		select {
		case exRes, ok = <-execResultCh:
			if !ok {
				// subscription ended
				return
			}
			writeCh <- message{
				Payload: json.RawMessage(exRes),
				ID:      id,
				Type:    msgNext,
			}
		case errCh, ok = <-errCh:
			if !ok {
				// subscription ended
				return
			}
			gqlErr := graphqlError{
				Message: "error in handleSubscription",
			}
			b, serErr := json.Marshal(gqlErr)
			if serErr != nil {
				panic(serErr)
			}
			writeCh <- message{
				Payload: b,
				ID:      id,
				Type:    msgError,
			}
			return
		}
	}
}
