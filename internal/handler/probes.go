package handler

import (
	"context"
	"net/http"
	"time"
)

func (h *Proxy) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func (h *Proxy) Ready() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.rdcl == nil {
			http.Error(w, "redis not configured", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := h.rdcl.Ping(ctx).Err(); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}
