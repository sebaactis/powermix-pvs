// Mock GS: recibe el POST notify del bridge y lo loguea.
// No valida firma (mock local). Devuelve envelope GS exitoso.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("MOCK_GS_ADDR")
	if addr == "" {
		addr = ":8082"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		log.Printf("[Mock GS] %s %s | key=%s key-md5=%s timestamp=%s",
			r.Method, r.URL.Path,
			r.Header.Get("key"), r.Header.Get("key-md5"), r.Header.Get("timestamp"))
		var pretty map[string]interface{}
		if json.Unmarshal(body, &pretty) == nil {
			pb, _ := json.MarshalIndent(pretty, "", "  ")
			log.Printf("[Mock GS] body:\n%s", string(pb))
		} else {
			log.Printf("[Mock GS] body: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200, "msg": "success",
			"data": map[string]string{"returnCode": "success", "returnMsg": "success"},
		})
	})
	log.Printf("Mock GS escuchando en http://%s ...", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("error en mock GS: %v", err)
	}
}
