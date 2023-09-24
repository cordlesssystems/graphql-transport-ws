package gqlgen

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/99designs/gqlgen/graphql"
	graphql_transport_ws "github.com/cordlesssystems/graphql-transport-ws"
	"io"
	"net/http"
)

type Transport struct {
	handleInit graphql_transport_ws.HandleInitFunc
}

func (t *Transport) Supports(r *http.Request) bool {
	return true
}

func (t *Transport) Do(w http.ResponseWriter, r *http.Request, exec graphql.GraphExecutor) {

	h, err := graphql_transport_ws.NewHandler(
		subscribe(exec),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.ServeHTTP(w, r)

}

func subscribe(exec graphql.GraphExecutor) graphql_transport_ws.HandleSubscribeFunc {
	return func(ctx context.Context, operationId string, payload json.RawMessage) (<-chan graphql_transport_ws.ExecutionResult, <-chan error, error) {
		ctx = graphql.StartOperationTrace(ctx)
		var params *graphql.RawParams
		if err := jsonDecode(bytes.NewReader(payload), &params); err != nil {
			return nil, nil, err
		}

		start := graphql.Now()
		params.ReadTime = graphql.TraceTiming{
			Start: start,
			End:   graphql.Now(),
		}

		rc, err := exec.CreateOperationContext(ctx, params)
		if err != nil {
			resp := exec.DispatchError(graphql.WithOperationContext(ctx, rc), err)
			return nil, nil, resp.Errors
		}

		exResCh := make(chan graphql_transport_ws.ExecutionResult)
		errCh := make(chan error)

		go func() {
			defer close(exResCh)
			defer close(errCh)

			var (
				responses  graphql.ResponseHandler
				execResult json.RawMessage
				serErr     error
			)
			responses, ctx = exec.DispatchOperation(ctx, rc)
			for {
				response := responses(ctx)
				if response == nil {
					break
				}

				execResult, serErr = json.Marshal(response)

				if serErr != nil {
					errCh <- serErr
					return
				}

				exResCh <- graphql_transport_ws.ExecutionResult(execResult)

			}

		}()

		return exResCh, errCh, nil
	}
}

func jsonDecode(r io.Reader, val interface{}) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec.Decode(val)
}
