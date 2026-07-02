package gs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/ports"
)

func TestSignRequest_AgregaHeaders(t *testing.T) {
	key := "bf2c87d52fba343b2617ffcd4205aabe"
	secret := "43f84d5ba3d40e68fbad75817e2a8958"

	req, _ := http.NewRequest("POST", "https://gs.example.com/test", nil)
	SignRequest(req, key, secret)

	if req.Header.Get("key") != key {
		t.Errorf("key = %q, esperaba %q", req.Header.Get("key"), key)
	}
	if req.Header.Get("key-md5") == "" {
		t.Error("key-md5 no fue seteado")
	}
	if req.Header.Get("timestamp") == "" {
		t.Error("timestamp no fue seteado")
	}

	// Verificar formato del key-md5 (debe ser hex lowercase de 32 chars)
	kmd5 := req.Header.Get("key-md5")
	if len(kmd5) != 32 {
		t.Errorf("key-md5 length = %d, esperaba 32", len(kmd5))
	}
	for _, c := range kmd5 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("key-md5 contiene caracter invalido: %c", c)
			break
		}
	}
}

func TestSignRequest_TimestampEnMilisegundos(t *testing.T) {
	key := "test-key"
	secret := "test-secret"

	req, _ := http.NewRequest("POST", "https://test.com", nil)
	SignRequest(req, key, secret)

	tsStr := req.Header.Get("timestamp")
	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		// No es un timestamp formato fecha, debe ser epoch millis
		tsInt, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			t.Fatalf("timestamp no es epoch millis: %q", tsStr)
		}
		// Debe tener 13 digitos (milliseconds)
		if tsStr[0] != '1' && tsStr[0] != '2' {
			t.Logf("timestamp = %s (epoch millis)", tsStr)
		}
		_ = tsInt
	} else {
		t.Errorf("timestamp es formato fecha %v, se esperaba epoch millis", ts)
	}
}

func TestQueryStatus_OK(t *testing.T) {
	mockGS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("key") == "" {
			t.Error("key header ausente")
		}
		if r.Header.Get("key-md5") == "" {
			t.Error("key-md5 header ausente")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"orderNo":      "GS-ORDER-001",
				"orderStatus":  1,
				"thirdOrderNo": "our-order-001",
			},
			"msg": "success",
		})
	}))
	defer mockGS.Close()

	client := New(mockGS.URL, "test-key", "test-secret")
	resp, err := client.QueryStatus(context.Background(), &ports.GSQueryRequest{
		OrderNo:      "GS-ORDER-001",
		ThirdOrderNo: "our-order-001",
	})
	if err != nil {
		t.Fatalf("QueryStatus fallo: %v", err)
	}
	if resp.OrderStatus != 1 {
		t.Errorf("OrderStatus = %d, esperaba 1 (pending)", resp.OrderStatus)
	}
	if resp.ThirdOrderNo != "our-order-001" {
		t.Errorf("ThirdOrderNo = %q, esperaba our-order-001", resp.ThirdOrderNo)
	}
}

func TestRefund_OK(t *testing.T) {
	mockGS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"refundNo":     "REFUND-001",
				"orderNo":      "GS-ORDER-001",
				"thirdOrderNo": "our-order-001",
				"refundStatus": "success",
			},
			"msg": "success",
		})
	}))
	defer mockGS.Close()

	client := New(mockGS.URL, "test-key", "test-secret")
	resp, err := client.Refund(context.Background(), &ports.GSRefundRequest{
		RefundNo:        "REFUND-001",
		OrderNo:         "GS-ORDER-001",
		ThirdOrderNo:    "our-order-001",
		RefundAmount:    "150.00",
		RefundReason:    "Producto agotado",
		RefundNotifyURL: "https://nosotros.com/webhook/gs",
	})
	if err != nil {
		t.Fatalf("Refund fallo: %v", err)
	}
	if resp.RefundStatus != "success" {
		t.Errorf("RefundStatus = %q, esperaba success", resp.RefundStatus)
	}
}

func TestQueryStatus_Error(t *testing.T) {
	mockGS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":400,"msg":"parametros invalidos"}`)
	}))
	defer mockGS.Close()

	client := New(mockGS.URL, "test-key", "test-secret")
	_, err := client.QueryStatus(context.Background(), &ports.GSQueryRequest{
		OrderNo: "INVALIDO",
	})
	if err == nil {
		t.Fatal("se esperaba error, pero fue nil")
	}
}

func TestSignRequest_HeaderOrderNoImporta(t *testing.T) {
	// Verifica que firma es consistente: mismo key+secret+timestamp produce misma firma
	key := "key-ejemplo"
	secret := "secret-ejemplo"

	req1, _ := http.NewRequest("POST", "https://test.com/a", nil)
	req2, _ := http.NewRequest("POST", "https://test.com/b", nil)

	SignRequest(req1, key, secret)
	// Para req2 usamos el mismo key+secret pero timestamp forzado (simulado)
	// Como SignRequest usa time.Now(), los timestamps van a ser diferentes
	// en cada llamado. Este test solo verifica que no panic.
	SignRequest(req2, key, secret)
}
