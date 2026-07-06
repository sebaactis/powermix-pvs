package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

type Transaction struct {
	QrID    string `json:"qrId"`
	StateID int    `json:"stateId"` // 6=In Process, 5=Approved, 4=Reverse, 3=Rejected
}

var (
	mu           sync.Mutex
	transactions = make(map[string]*Transaction)
)

func main() {
	mux := http.NewServeMux()

	// OAuth2 token mock
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token-abc-123",
			"expires_in":   3600,
		})
	})

	// Generate QR mock
	mux.HandleFunc("POST /external/connect/api/v1/qr/pvs", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Amount     string `json:"amount"`
			ExternalID string `json:"externalId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		qrID := "qr-" + req.ExternalID
		mu.Lock()
		transactions[qrID] = &Transaction{
			QrID:    qrID,
			StateID: 6, // IN_PROCESS
		}
		mu.Unlock()

		log.Printf("[PVS Mock] QR Generado para la orden %s, QR_ID: %s, Monto: %s\n", req.ExternalID, qrID, req.Amount)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"qrId":    qrID,
			"qrImage": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=", // 1x1 pixel dummy base64
		})
	})

	// Query Status mock (usado por el Reconciler)
	mux.HandleFunc("GET /external/connect/api/v1/transactions/qrpvs/{qrId}", func(w http.ResponseWriter, r *http.Request) {
		qrID := r.PathValue("qrId")
		mu.Lock()
		t, ok := transactions[qrID]
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Printf("[PVS Mock] Query para QR_ID %s: StateID %d\n", qrID, t.StateID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stateId": t.StateID,
		})
	})

	// Reverse mock (reembolso)
	mux.HandleFunc("POST /external/connect/api/v1/qr/pvs/reverse", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			QrID string `json:"qrId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		mu.Lock()
		t, ok := transactions[req.QrID]
		if ok {
			t.StateID = 4 // REVERSED (Reembolsado)
		}
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Printf("[PVS Mock] Reverso para QR_ID %s ejecutado con éxito\n", req.QrID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	})

	// Admin endpoint: para cambiar manualmente el estado en PVS desde afuera
	mux.HandleFunc("POST /admin/transactions/{qrId}/status", func(w http.ResponseWriter, r *http.Request) {
		qrID := r.PathValue("qrId")
		var req struct {
			StateID int `json:"stateId"` // 5=Approved, 3=Rejected, etc.
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		mu.Lock()
		t, ok := transactions[qrID]
		if ok {
			t.StateID = req.StateID
		}
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Printf("[PVS Mock Admin] QR_ID %s forzado a StateID %d\n", qrID, req.StateID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"qrId":    qrID,
			"stateId": t.StateID,
		})
	})

	// Logger middleware simple
	loggedMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})

	log.Println("Mock PVS Server escuchando en http://localhost:8081...")
	if err := http.ListenAndServe(":8081", loggedMux); err != nil {
		log.Fatalf("error en el mock server: %v", err)
	}
}
