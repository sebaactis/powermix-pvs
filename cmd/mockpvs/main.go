package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

// Transaction guarda el estado de un QR en el mock (memoria).
// stateId: 6=In Process, 5=Approved, 4=Reverse, 3=Rejected (doc Get QR Status).
type Transaction struct {
	QrID       string
	StateID    int
	Reference  string
	ExternalID string
}

var (
	mu           sync.Mutex
	transactions = make(map[string]*Transaction)
)

func writeOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    "OK",
		"message": "Operacion exitosa.",
		"ok":      true,
		"data":    data,
	})
}

func stateLabel(stateID int) string {
	switch stateID {
	case 6:
		return "Procesando"
	case 5:
		return "Approved"
	case 4:
		return "Reverse"
	case 3:
		return "Rejected"
	default:
		return "Unknown"
	}
}

func main() {
	mux := http.NewServeMux()

	// OAuth2 token mock (Basic + grant_type; el mock no valida credenciales).
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token-abc-123",
			"expires_in":   3600,
		})
	})

	// Doc body: amount, externalId, reference. Response data: qrId, qrImage, qrRaw...
	mux.HandleFunc("POST /external/connect/api/v1/qr/pvs", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Amount     string `json:"amount"`
			ExternalID string `json:"externalId"`
			Reference  string `json:"reference"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		qrID := "qr-" + req.ExternalID
		mu.Lock()
		transactions[qrID] = &Transaction{
			QrID:       qrID,
			StateID:    6, // IN_PROCESS
			Reference:  req.Reference,
			ExternalID: req.ExternalID,
		}
		mu.Unlock()

		log.Printf("[PVS Mock] QR generado externalId=%s qrId=%s amount=%s ref=%s\n",
			req.ExternalID, qrID, req.Amount, req.Reference)

		// 1x1 PNG dummy base64
		const dummyImg = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
		writeOK(w, map[string]interface{}{
			"qrId":    qrID,
			"qrImage": dummyImg,
			"qrRaw":   "000201mock",
		})
	})

	// Get QR Status: GET /external/connect/api/v1/transactions/qrpvs/{qrId}
	mux.HandleFunc("GET /external/connect/api/v1/transactions/qrpvs/{qrId}", func(w http.ResponseWriter, r *http.Request) {
		qrID := r.PathValue("qrId")
		mu.Lock()
		t, ok := transactions[qrID]
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Printf("[PVS Mock] Query qrId=%s stateId=%d\n", qrID, t.StateID)

		writeOK(w, map[string]interface{}{
			"qrId":    t.QrID,
			"stateId": t.StateID,
			"state":   stateLabel(t.StateID),
		})
	})

	// Reverse: POST /external/connect/api/v1/qr/pvs/reverse
	// Doc body: { qrId }. Response data: txeId, qrId, authorizationCode.
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
			t.StateID = 4 // REVERSED
		}
		mu.Unlock()

		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Printf("[PVS Mock] Reverse qrId=%s ok\n", req.QrID)

		writeOK(w, map[string]interface{}{
			"txeId":             "txe-mock-" + req.QrID,
			"qrId":              req.QrID,
			"authorizationCode": "MOCK01",
		})
	})

	// Admin: fuerza stateId (reconciler / pruebas sin webhook).
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

		log.Printf("[PVS Mock Admin] qrId=%s stateId=%d\n", qrID, t.StateID)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"qrId":    qrID,
			"stateId": t.StateID,
		})
	})

	loggedMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})

	log.Println("Mock PVS Server escuchando en http://localhost:8081...")
	if err := http.ListenAndServe(":8081", loggedMux); err != nil {
		log.Fatalf("error en el mock server: %v", err)
	}
}
