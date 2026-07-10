package gs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		_, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			t.Fatalf("timestamp no es epoch millis: %q", tsStr)
		}
	} else {
		t.Errorf("timestamp es formato fecha %v, se esperaba epoch millis", ts)
	}
}

func TestNotifyPayment_OK(t *testing.T) {
	var gotBody map[string]string
	mockGS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("key") == "" {
			t.Error("key header ausente")
		}
		if r.Header.Get("key-md5") == "" {
			t.Error("key-md5 header ausente")
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"msg":  "success",
			"data": map[string]string{
				"returnCode": "success",
				"returnMsg":  "ok",
			},
		})
	}))
	defer mockGS.Close()

	client := New("https://unused.example", "test-key", "test-secret")
	resp, err := client.NotifyPayment(context.Background(), &ports.GSNotifyPaymentRequest{
		NotifyURL:     mockGS.URL + "/pay/notify",
		OrderNo:       "GS-ORDER-001",
		ThirdOrderNo:  "our-order-001",
		OrderStatus:   "2",
		OrderTime:     "2026-03-01 18:28:16",
		PayTime:       "2026-03-01 18:30:14",
		TotalAmount:   "15.00",
		ChannelUserID: "",
	})
	if err != nil {
		t.Fatalf("NotifyPayment fallo: %v", err)
	}
	if resp.ReturnCode != "success" {
		t.Errorf("ReturnCode = %q, esperaba success", resp.ReturnCode)
	}
	if gotBody["orderStatus"] != "2" {
		t.Errorf("body orderStatus = %q, esperaba 2", gotBody["orderStatus"])
	}
	if gotBody["thirdOrderNo"] != "our-order-001" {
		t.Errorf("body thirdOrderNo = %q", gotBody["thirdOrderNo"])
	}
}

func TestNotifyPayment_HTTPError(t *testing.T) {
	mockGS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":400,"msg":"fail"}`)
	}))
	defer mockGS.Close()

	client := New("https://unused.example", "test-key", "test-secret")
	_, err := client.NotifyPayment(context.Background(), &ports.GSNotifyPaymentRequest{
		NotifyURL:    mockGS.URL,
		OrderNo:      "X",
		ThirdOrderNo: "Y",
		OrderStatus:  "2",
	})
	if err == nil {
		t.Fatal("se esperaba error, pero fue nil")
	}
}

func TestNotifyPayment_URLObligatoria(t *testing.T) {
	client := New("https://unused.example", "test-key", "test-secret")
	_, err := client.NotifyPayment(context.Background(), &ports.GSNotifyPaymentRequest{})
	if err == nil {
		t.Fatal("se esperaba error por notifyUrl vacia")
	}
}

func TestSignRequest_HeaderOrderNoImporta(t *testing.T) {
	key := "key-ejemplo"
	secret := "secret-ejemplo"

	req1, _ := http.NewRequest("POST", "https://test.com/a", nil)
	req2, _ := http.NewRequest("POST", "https://test.com/b", nil)

	SignRequest(req1, key, secret)
	SignRequest(req2, key, secret)
}
