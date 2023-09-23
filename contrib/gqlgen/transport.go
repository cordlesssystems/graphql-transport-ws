package gqlgen

import (
	"github.com/99designs/gqlgen/graphql"
	"net/http"
)

type Transport struct {
}

func (t *Transport) Supports(r *http.Request) bool {
	return true
}

func (t *Transport) Do(w http.ResponseWriter, r *http.Request, exec graphql.GraphExecutor) {
	//TODO implement me
	panic("implement me")
}
