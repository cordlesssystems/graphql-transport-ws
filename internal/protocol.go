package internal

import (
	"encoding/json"
	"github.com/gorilla/websocket"
)

// https://github.com/enisdenjo/graphql-ws/blob/master/PROTOCOL.md

type messageType string

const (
	msgConnectionInit = "connection_init" // Client -> Server
	msgConnectionAck  = "connection_ack"  // Server -> Client
	msgPing           = "ping"            // bidirectional
	msgPong           = "pong"            // bidirectional
	msgSubscribe      = "subscribe"       // Client -> Server
	msgNext           = "next"            // Server -> Client
	msgError          = "error"           // Server -> Client
	msgComplete       = "complete"        // bidirectional
)

type message struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	ID      string          `json:"id,omitempty"`
	Type    messageType     `json:"type"`
}

type location struct {
	Line   string `json:"line"`
	Column string `json:"column"`
}

type graphqlError struct {
	Message    string                 `json:"message"`
	Locations  []location             `json:"locations,omitempty"`
	Path       []string               `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

type GenericPayload map[string]interface{}

var (
	closeInitTimeout        = websocket.FormatCloseMessage(4408, "Connection initialisation timeout")
	closeMessageParsing     = websocket.FormatCloseMessage(4400, "Message parsing error")
	closeForbidden          = websocket.FormatCloseMessage(4403, "Forbidden")
	closeUnknownMessageType = websocket.FormatCloseMessage(4400, "Unknown message type")
	closeTooManyInit        = websocket.FormatCloseMessage(4429, "Too many initialisation requests")
)
