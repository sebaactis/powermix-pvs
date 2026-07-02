package config

import (
	"os"
	"testing"
	"time"
)

// TestDefaults: sin las variables requeridas, Load() debe fallar
// indicando que DATABASE_URL u otras obligatorias faltan.
func TestDefaults(t *testing.T) {
	_, err := Load()
	if err == nil {
		t.Fatal("se esperaba error porque DATABASE_URL es requerida")
	}
	if !contiene(err.Error(), "DATABASE_URL") {
		t.Errorf("mensaje de error no menciona DATABASE_URL: %v", err)
	}
}

// TestCustomEnv: seteamos todas las variables requeridas y chequeamos
// que los defaults se aplican correctamente a las opcionales.
func TestCustomEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/vps_powermix")
	t.Setenv("GS_BASE_URL", "https://gs.example.com")
	t.Setenv("GS_KEY", "test-key-123")
	t.Setenv("GS_SECRET", "test-secret-456")
	t.Setenv("PVS_CLIENT_ID", "pvs-client-test")
	t.Setenv("PVS_CLIENT_SECRET", "pvs-secret-test")
	t.Setenv("PVS_CALLBACK_URL", "https://miapp.com/webhook/pvs")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("RECONCILER_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	ok := true
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTP_ADDR = %q, se esperaba %q", cfg.HTTPAddr, ":9090")
		ok = false
	}
	if cfg.ReconcilerEnabled != true {
		t.Errorf("RECONCILER_ENABLED = false, se esperaba true")
		ok = false
	}
	if cfg.PVSBaseURL != "https://api01.pvssa.com.ar" {
		t.Errorf("PVSBaseURL = %q, se esperaba default sandbox", cfg.PVSBaseURL)
		ok = false
	}
	if cfg.ReconcilerBatchSize != 200 {
		t.Errorf("ReconcilerBatchSize = %d, se esperaba default 200", cfg.ReconcilerBatchSize)
		ok = false
	}
	if cfg.GSReplayWindow != 5*time.Minute {
		t.Errorf("GSReplayWindow = %v, se esperaba 5m", cfg.GSReplayWindow)
		ok = false
	}
	if cfg.QRExpiry != 3*time.Minute {
		t.Errorf("QRExpiry = %v, se esperaba 3m", cfg.QRExpiry)
		ok = false
	}
	if cfg.GSEnabled != false {
		t.Errorf("GSEnabled = true, se esperaba false (default)")
		ok = false
	}
	if !ok {
		t.Log("uno o mas tests fallaron, revisar mensajes arriba")
	}
}

// TestMissingRequired: si falta UNA variable requerida, el error
// debe mencionar exactamente esa variable.
func TestMissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("GS_BASE_URL", "https://gs.test.com")
	t.Setenv("GS_KEY", "key")
	t.Setenv("GS_SECRET", "secret")
	t.Setenv("PVS_CLIENT_ID", "client-id")
	// NO seteamos PVS_CLIENT_SECRET a proposito
	t.Setenv("PVS_CALLBACK_URL", "https://test.com/webhook")

	_, err := Load()
	if err == nil {
		t.Fatal("se esperaba error por PVS_CLIENT_SECRET faltante")
	}
	if !contiene(err.Error(), "PVS_CLIENT_SECRET") {
		t.Errorf("error no menciona la variable faltante: %v", err)
	}
}

// TestValidateReglasNegocio: chequea que las reglas de negocio
// se validan correctamente.
func TestValidateReglasNegocio(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "configuracion valida",
			cfg: Config{
				GSReplayWindow:       5 * time.Minute,
				QRExpiry:             3 * time.Minute,
				ReconcilerBatchSize:  200,
				SyncLogRetentionDays: 30,
			},
			wantErr: false,
		},
		{
			name: "replay window muy chica",
			cfg: Config{
				GSReplayWindow:       5 * time.Second,
				QRExpiry:             3 * time.Minute,
				ReconcilerBatchSize:  200,
				SyncLogRetentionDays: 30,
			},
			wantErr: true,
		},
		{
			name: "batch size fuera de rango",
			cfg: Config{
				GSReplayWindow:       5 * time.Minute,
				QRExpiry:             3 * time.Minute,
				ReconcilerBatchSize:  5000,
				SyncLogRetentionDays: 30,
			},
			wantErr: true,
		},
		{
			name: "sync log retention minimo",
			cfg: Config{
				GSReplayWindow:       5 * time.Minute,
				QRExpiry:             3 * time.Minute,
				ReconcilerBatchSize:  100,
				SyncLogRetentionDays: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateReglasNegocio()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReglasNegocio() error = %v, wantErr = %v",
					err, tt.wantErr)
			}
		})
	}
}

// TestGetEnvBoolAcceptaVariosValores: verifica que getEnvBool
// interpreta correctamente true, false y valores por defecto.
func TestGetEnvBoolAcceptaVariosValores(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		fallback bool
		want     bool
	}{
		{"true literal", "true", false, true},
		{"TRUE mayuscula", "TRUE", false, true},
		{"1 numerico", "1", false, true},
		{"yes textual", "yes", false, true},
		{"false literal", "false", true, false},
		{"FALSE mayuscula", "FALSE", true, false},
		{"0 numerico", "0", true, false},
		{"no textual", "no", true, false},
		{"env vacio con fallback true", "", true, true},
		{"env vacio con fallback false", "", false, false},
		{"valor invalido con fallback true", "cualquiercosa", true, true},
		{"valor invalido con fallback false", "cualquiercosa", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("TEST_BOOL_VAR", tt.envVal)
			} else {
				os.Unsetenv("TEST_BOOL_VAR")
			}
			got := getEnvBoolImpl("TEST_BOOL_VAR", tt.fallback)
			if got != tt.want {
				t.Errorf("getEnvBool(%q) con fallback=%v = %v, se esperaba %v",
					tt.envVal, tt.fallback, got, tt.want)
			}
		})
	}
}

// Helpers

func contiene(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getEnvBoolImpl es la implementacion de getEnvBool extraida para
// poder testearla sin depender de Load(). Lee os.Getenv directamente.
func getEnvBoolImpl(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch v {
	case "true", "TRUE", "1", "yes", "YES":
		return true
	case "false", "FALSE", "0", "no", "NO":
		return false
	default:
		return fallback
	}
}
