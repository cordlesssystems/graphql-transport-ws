package internal

import "time"

func WithHandleInit(handleInit HandleInitFunc) func(*Handler) {
	return func(h *Handler) {
		h.handleInit = handleInit
	}
}

func WithConnectionInitTimeout(timeout time.Duration) func(*Handler) {
	return func(h *Handler) {
		h.connectionInitWaitTimeout = timeout
	}
}

func WithDebug() func(*Handler) {
	return func(h *Handler) {
		h.debugHandler = true
	}
}
