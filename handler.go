package graphql_transport_ws

import (
	"context"
	"encoding/json"
	"github.com/cordlesssystems/graphql-transport-ws/internal"
	"net/http"
)

type (
	ExecutionResult json.RawMessage
)

type Location struct {
	Line   string `json:"line"`
	Column string `json:"column"`
}

type GraphqlError struct {
	Message    string                 `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Path       []string               `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

type (
	InitPayload    map[string]interface{}
	InitAckPayload map[string]interface{}
)

type (
	HandleInitFunc      = func(ctx context.Context, payload InitPayload) (context.Context, json.RawMessage, error)
	HandleSubscribeFunc = func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan ExecutionResult, <-chan error, error)
)

type handler struct {
	handler *internal.Handler
}

type NewHandlerOpts struct {
	HandleInit HandleInitFunc
}

func NewHandler(
	handleSubscribe HandleSubscribeFunc,
	opts ...func(*NewHandlerOpts),
) (http.Handler, error) {
	handlerOpts := &NewHandlerOpts{}
	for _, opt := range opts {
		opt(handlerOpts)
	}
	var (
		internalHandler *internal.Handler
		err             error
	)
	if handlerOpts.HandleInit == nil {
		internalHandler, err = internal.NewHandler(getSubscribeFunc(handleSubscribe))
		if err != nil {
			return nil, err
		}
	} else {
		internalHandler, err = internal.NewHandler(getSubscribeFunc(handleSubscribe), internal.WithHandleInit(getInitFunc(handlerOpts.HandleInit)))
		if err != nil {
			return nil, err
		}
	}
	return &handler{
		handler: internalHandler,
	}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func getInitFunc(handleInit HandleInitFunc) internal.HandleInitFunc {
	return func(ctx context.Context, payload internal.GenericPayload) (context.Context, json.RawMessage, error) {
		var (
			newCtx     context.Context
			ackPayload json.RawMessage
			err        error
		)
		newCtx, ackPayload, err = handleInit(ctx, InitPayload(payload))
		return newCtx, ackPayload, err
	}
}

func getSubscribeFunc(handleSubscribe HandleSubscribeFunc) internal.HandleSubscribeFunc {
	return func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {

		var (
			exResCh <-chan ExecutionResult
			errCh   <-chan error
			err     error

			exRes ExecutionResult
			ok    bool
		)

		exResCh, errCh, err = handleSubscribe(ctx, operationId, payload)

		if err != nil {
			return nil, nil, err
		}

		exResChInternal := make(chan internal.ExecutionResult)
		errChInternal := make(chan error)

		go func() {
			defer close(exResChInternal)
			defer close(errChInternal)

			for {
				select {
				case exRes, ok = <-exResCh:
					if !ok {
						return
					}
					exResChInternal <- internal.ExecutionResult(exRes)
				case err, ok = <-errCh:
					if !ok {
						return
					}
					errChInternal <- err

				}
			}
		}()

		return exResChInternal, errChInternal, nil

	}
}
