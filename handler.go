package graphql_transport_ws

import (
	"github.com/cordlesssystems/graphql-transport-ws/internal"
	"net/http"
)

type handler struct {
	handler *internal.Handler
}

func NewHandler() http.Handler {
	return &handler{
		handler: internal.NewHandler(),
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}
