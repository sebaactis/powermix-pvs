package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// TestMigrationsAplicanCorrectamente verifica que las 3 migraciones
// se aplican y revierten sin errores contra una base de datos real.
// Salta si no hay DATABASE_URL disponible (desarrollo local sin Postgres).
func TestMigrationsAplicanCorrectamente(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL no configurada, saltando test de migraciones")
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("error conectando a la base de datos: %v", err)
	}
	defer db.Close()

	// Migraciones UP en orden
	migrations := []struct {
		nombre string
		sql    string
	}{
		{"001_orders", "./001_orders.up.sql"},
		{"002_idempotency_keys", "./002_idempotency_keys.up.sql"},
		{"003_api_sync_log", "./003_api_sync_log.up.sql"},
		{"004_refunds", "./004_refunds.up.sql"},
		{"005_reconciler_runs", "./005_reconciler_runs.up.sql"},
		{"006_order_status_history", "./006_order_status_history.up.sql"},
		{"007_gs_v2_fields", "./007_gs_v2_fields.up.sql"},
		{"008_rename_to_third_order_no", "./008_rename_to_third_order_no.up.sql"},
	}

	for _, m := range migrations {
		t.Run(m.nombre, func(t *testing.T) {
			sql, err := os.ReadFile(m.sql)
			if err != nil {
				t.Fatalf("no se pudo leer %s: %v", m.sql, err)
			}
			_, err = db.Exec(string(sql))
			if err != nil {
				t.Fatalf("error ejecutando %s: %v", m.sql, err)
			}
			t.Logf("migracion %s ejecutada correctamente", m.nombre)
		})
	}

	// Verificar que las tablas existen
	var tablas []string
	err = db.Select(&tablas, `SELECT table_name FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name IN ('orders', 'idempotency_keys', 'api_sync_log', 'refunds', 'reconciler_runs', 'order_status_history') 
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("error consultando tablas: %v", err)
	}

	tablasEsperadas := map[string]bool{
		"orders":               false,
		"idempotency_keys":     false,
		"api_sync_log":         false,
		"refunds":              false,
		"reconciler_runs":      false,
		"order_status_history": false,
	}
	for _, t := range tablas {
		tablasEsperadas[t] = true
	}
	for nombre, encontrada := range tablasEsperadas {
		if !encontrada {
			t.Errorf("tabla %s no fue creada por las migraciones", nombre)
		}
	}

	// Migraciones DOWN en orden inverso
	down := []struct {
		nombre string
		sql    string
	}{
		{"008_rename_to_third_order_no", "./008_rename_to_third_order_no.down.sql"},
		{"007_gs_v2_fields", "./007_gs_v2_fields.down.sql"},
		{"006_order_status_history", "./006_order_status_history.down.sql"},
		{"005_reconciler_runs", "./005_reconciler_runs.down.sql"},
		{"004_refunds", "./004_refunds.down.sql"},
		{"003_api_sync_log", "./003_api_sync_log.down.sql"},
		{"002_idempotency_keys", "./002_idempotency_keys.down.sql"},
		{"001_orders", "./001_orders.down.sql"},
	}

	for _, m := range down {
		t.Run(m.nombre+"_down", func(t *testing.T) {
			sql, err := os.ReadFile(m.sql)
			if err != nil {
				t.Fatalf("error leyendo %s: %v", m.sql, err)
			}
			_, err = db.Exec(string(sql))
			if err != nil {
				t.Fatalf("error ejecutando down %s: %v", m.sql, err)
			}
		})
	}

	t.Log("migraciones up + down ejecutadas correctamente")
}
