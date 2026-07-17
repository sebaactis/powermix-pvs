package keepalive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRun_PingeaLaURLPeriodicamente: Run debe pegarle a la URL cada
// Interval hasta que el contexto se cancele. Usamos un httptest.Server
// con un contador atomico y un intervalo corto para que el test sea
// rapido y no-flaky.
func TestRun_PingeaLaURLPeriodicamente(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx, Options{URL: srv.URL, Interval: 10 * time.Millisecond})
		close(done)
	}()

	// Esperamos a que pegue al menos 3 veces (con margen generoso).
	deadline := time.After(2 * time.Second)
	for {
		if hits.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: solo %d hits, se esperaban >= 3", hits.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	cancel()
	<-done // Run debe terminar tras cancelar el contexto
}

// TestRun_SalidaInmediataSinConfig: sin URL o con intervalo <= 0,
// Run debe retornar de inmediato sin hacer nada.
func TestRun_SalidaInmediataSinConfig(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"url vacia", Options{URL: "", Interval: 30 * time.Second}},
		{"intervalo cero", Options{URL: "https://example.com/healthz", Interval: 0}},
		{"intervalo negativo", Options{URL: "https://example.com/healthz", Interval: -1 * time.Second}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				Run(context.Background(), tc.opts)
				close(done)
			}()
			select {
			case <-done:
				// ok: salio de inmediato
			case <-time.After(200 * time.Millisecond):
				t.Fatal("Run no termino de inmediato con config invalida")
			}
		})
	}
}

// TestRun_NoPanicaAnteStatusNo2xx: si el endpoint responde con error,
// Run debe seguir vivo (loggea pero no panic) y seguir tickeando.
func TestRun_NoPanicaAnteStatusNo2xx(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		Run(ctx, Options{URL: srv.URL, Interval: 10 * time.Millisecond})
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for {
		if hits.Load() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: solo %d hits, se esperaban >= 2 (no debio panic)", hits.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	cancel()
	<-done
}
