package logging

import "net/http"

const RequestIDHeader = "X-Request-ID"

// RequestIDMiddleware lee X-Request-ID del header entrante.
// Si está vacío, genera uno nuevo con NewRequestID().
// Inyecta el ID en el contexto vía WithRequestID y hace eco en
// el response header X-Request-ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = NewRequestID()
		}

		w.Header().Set(RequestIDHeader, id)
		ctx := WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
