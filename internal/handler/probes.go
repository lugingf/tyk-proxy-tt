package handler

import "net/http"

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

		if err := h.rdcl.Ping(r.Context()).Err(); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}
