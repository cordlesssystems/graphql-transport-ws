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
	HandleInitFunc      = func(ctx context.Context, payload InitPayload) (context.Context, InitAckPayload, error)
	HandleSubscribeFunc = func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan ExecutionResult, <-chan error, error)
)

type handler struct {
	handler *internal.Handler
}

func NewHandler(
	handleInit HandleInitFunc,
	handleSubscribe HandleSubscribeFunc,
) http.Handler {
	return &handler{
		handler: internal.NewHandler(
			func(ctx context.Context, payload internal.GenericPayload) (context.Context, internal.GenericPayload, error) {
				var (
					newCtx     context.Context
					ackPayload InitAckPayload
					err        error
				)
				newCtx, ackPayload, err = handleInit(ctx, InitPayload(payload))
				return newCtx, internal.GenericPayload(ackPayload), err
			},
			func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan internal.ExecutionResult, <-chan error, error) {

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

			},
		),
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}
