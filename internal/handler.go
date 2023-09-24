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
	defaultInitTimeout    = time.Second * 5
	defaultControlTimeout = func() time.Time {
		return time.Now().Add(time.Second * 5)
	}
)

type (
	ExecutionResult json.RawMessage
)

type (
	HandleInitFunc      = func(ctx context.Context, payload GenericPayload) (context.Context, json.RawMessage, error)
	HandleSubscribeFunc = func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan ExecutionResult, <-chan error, error)
)

type Handler struct {
	upgrader                  *websocket.Upgrader
	connectionInitWaitTimeout time.Duration
	handleInit                HandleInitFunc
	handleSubscribe           HandleSubscribeFunc
}

type WebsocketCloser interface {
	Code() int
	Reason() string
}

func WithHandleInit(handleInit HandleInitFunc) func(*Handler) {
	return func(h *Handler) {
		h.handleInit = handleInit
	}
}

func NewHandler(handleSubscribe HandleSubscribeFunc, opts ...func(*Handler)) (*Handler, error) {

	if handleSubscribe == nil {
		return nil, errors.New("handleSubscribe is nil")
	}

	h := &Handler{
		handleInit:      handleInitDefault,
		handleSubscribe: handleSubscribe,
		upgrader: &websocket.Upgrader{
			Subprotocols: []string{"graphql-transport-ws"},
		},
		connectionInitWaitTimeout: defaultInitTimeout,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(w, r, http.Header{})
	if err != nil {
		panic(err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			panic(err)
		}
	}()

	// Run initialisation
	ctx := context.Background()

	messagesCh := make(chan message, 100)
	defer close(messagesCh)
	go func() {
		_ = runWriterLoop(conn, messagesCh)
	}()
	_ = runReaderLoop(ctx, conn, messagesCh, h.handleInit, h.handleSubscribe, h.connectionInitWaitTimeout) // blocking call
}

func runReaderLoop(
	ctx context.Context,
	conn *websocket.Conn,
	ch chan<- message,
	handleInit HandleInitFunc,
	handleSubscribe HandleSubscribeFunc,
	initTimeout time.Duration,
) error {

	var (
		reader                                  io.Reader
		err                                     error
		closeErr                                *websocket.CloseError
		msg                                     message
		opMu                                    sync.Mutex
		subscriptions                           = make(map[string]func())
		cancel                                  func()
		ok                                      bool
		initialised                             AtomicBool
		initialisedContext, subscriptionContext context.Context
		ackPayload                              json.RawMessage
	)

	timer := time.NewTimer(initTimeout)
	defer timer.Stop()
	go func() {
		<-timer.C
		if !initialised.Value() {
			err = conn.WriteControl(websocket.CloseMessage, closeInitTimeout, defaultControlTimeout())
			if err != nil {
				panic(err)
			}
		}
	}()

	var wg sync.WaitGroup
	defer func() {
		opMu.Lock()
		for _, cancel = range subscriptions {
			cancel()
		}
		opMu.Unlock()
		wg.Wait() // wait until all subscriptions are finished
	}()
	for {
		_, reader, err = conn.NextReader()
		if errors.As(err, &closeErr) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unexpected error in .NextReader(): %v", err)
		}

		err = json.NewDecoder(reader).Decode(&msg)
		if err != nil {
			err = conn.WriteControl(websocket.CloseMessage, closeMessageParsing, defaultControlTimeout())
			if err != nil {
				return fmt.Errorf("unable to write CloseMessage control %s: %v", closeMessageParsing, err)
			}
			continue
		}

		switch msg.Type {
		case msgConnectionInit:
			initialisedContext, ackPayload, err = initialise(ctx, initialised, conn, reader, handleInit)
			if err != nil {
				return err
			}
			initialised.Set(true)
			ch <- message{
				Payload: ackPayload,
				Type:    msgConnectionAck,
			}

		case msgPing:
			// implement me
		case msgPong:
			// not supported yet
		case msgComplete:
			opMu.Lock()
			cancel, ok = subscriptions[msg.ID]
			if ok {
				cancel()
			}
			opMu.Unlock()
		case msgSubscribe:
			if !initialised.Value() {
				// send error
				// but not close socket
			}

			// subscriptionContext will be canceled on "complete" message
			// for this subscription
			subscriptionContext, cancel = context.WithCancel(initialisedContext)
			opMu.Lock()
			subscriptions[msg.ID] = cancel
			opMu.Unlock()

			wg.Add(1)
			go func() {
				runSubscriptionLoop(subscriptionContext, msg.ID, msg.Payload, handleSubscribe, ch)
				wg.Done()
				opMu.Lock()
				delete(subscriptions, msg.ID)
				opMu.Unlock()
			}()
		default:
			// write close message with relevant code
		}

	}
}

func runWriterLoop(conn *websocket.Conn, ch <-chan message) error {
	var (
		msg message
		ok  bool
		w   io.WriteCloser
		err error
		b   []byte
	)

	for {
		msg, ok = <-ch
		if !ok {
			return nil
		}

		w, err = conn.NextWriter(websocket.TextMessage)
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil
		}
		if err != nil {
			return err
		}

		b, err = json.Marshal(msg)
		if err != nil {
			return err
		}

		_, err = w.Write(b)
		if err != nil {
			return err
		}

		err = w.Close()
		if err != nil {
			return err
		}

	}
}

func runSubscriptionLoop(
	ctx context.Context,
	id string,
	payload json.RawMessage,
	handleSubscribe HandleSubscribeFunc,
	writeCh chan<- message,
) {

	var (
		exRes    ExecutionResult
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
		case err, ok = <-errCh:
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

func initialise(
	ctx context.Context,
	initialised AtomicBool,
	conn *websocket.Conn,
	reader io.Reader,
	handleInit HandleInitFunc,
) (context.Context, json.RawMessage, error) {

	var (
		msg                message
		initPayload        GenericPayload
		initialisedContext context.Context
		ackPayload         json.RawMessage
		err                error
	)

	if initialised.Value() {
		err = conn.WriteControl(websocket.CloseMessage, closeTooManyInit, defaultControlTimeout())
		if err != nil {
			return ctx, nil, fmt.Errorf("unable to write CloseMessage control %s: %v", closeTooManyInit, err)
		}
		return ctx, nil, errors.New("already initialised")
	}
	err = json.NewDecoder(reader).Decode(&msg)
	if err != nil {
		err = conn.WriteControl(websocket.CloseMessage, closeMessageParsing, defaultControlTimeout())
		if err != nil {
			return ctx, nil, fmt.Errorf("unable to write CloseMessage control %s: %v", closeMessageParsing, err)
		}
		return ctx, nil, errors.New("unable parse message")
	}
	err = json.Unmarshal(msg.Payload, &initPayload)
	if err != nil {
		err = conn.WriteControl(websocket.CloseMessage, closeMessageParsing, defaultControlTimeout())
		if err != nil {
			return ctx, nil, fmt.Errorf("unable to write CloseMessage control %s: %v", closeMessageParsing, err)
		}
		return ctx, nil, errors.New("unable parse init payload")
	}
	initialisedContext, ackPayload, err = handleInit(ctx, initPayload)
	if err != nil {
		var closeMsg []byte
		if e, ok := err.(WebsocketCloser); ok { // init handler might provide custom close message
			closeMsg = websocket.FormatCloseMessage(e.Code(), e.Reason())
		} else {
			closeMsg = closeForbidden
		}

		err = conn.WriteControl(websocket.CloseMessage, closeMsg, defaultControlTimeout())
		if err != nil {
			return ctx, nil, fmt.Errorf("unable to write CloseMessage control after init function %s: %v", closeMsg, err)
		}
		return ctx, nil, errors.New("initFunc returned error")
	}

	return initialisedContext, ackPayload, nil
}

func handleInitDefault(ctx context.Context, payload GenericPayload) (context.Context, json.RawMessage, error) {
	return ctx, nil, nil
}
