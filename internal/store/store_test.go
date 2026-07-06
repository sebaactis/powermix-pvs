package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// TestPostgresOrderRepository_CreateEGetByOrderNo verifica que se puede
// crear una orden y recuperarla por su order_no.
func TestPostgresOrderRepository_CreateEGetByOrderNo(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresOrderRepository(db)
	ctx := context.Background()

	// Crear orden
	orden := &domain.Order{
		OrderNo:    "test-order-001",
		DeviceID:   "dev-001",
		ObjectID:   "drink-fresa",
		PriceCents: 15000,
		Status:     domain.OrderReceived,
	}

	err := repo.Create(ctx, orden)
	if err != nil {
		t.Fatalf("error creando orden: %v", err)
	}

	if orden.ID == 0 {
		t.Error("ID no fue asignado por la base de datos")
	}
	if orden.CreatedAt.IsZero() {
		t.Error("CreatedAt no fue asignado")
	}

	// Recuperar por order_no
	recuperada, err := repo.GetByOrderNo(ctx, orden.OrderNo)
	if err != nil {
		t.Fatalf("error recuperando orden: %v", err)
	}

	if recuperada.OrderNo != orden.OrderNo {
		t.Errorf("order_no = %q, esperaba %q", recuperada.OrderNo, orden.OrderNo)
	}
	if recuperada.PriceCents != 15000 {
		t.Errorf("PriceCents = %d, esperaba 15000", recuperada.PriceCents)
	}
	if recuperada.Status != domain.OrderReceived {
		t.Errorf("Status = %q, esperaba RECEIVED", recuperada.Status)
	}
}

// TestPostgresOrderRepository_UpdateStatus verifica que se puede
// actualizar el estado de una orden.
func TestPostgresOrderRepository_UpdateStatus(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresOrderRepository(db)
	ctx := context.Background()

	orden := crearOrdenTest(t, repo, ctx)

	err := repo.UpdateStatus(ctx, orden.OrderNo, domain.OrderQRRequested)
	if err != nil {
		t.Fatalf("error actualizando estado: %v", err)
	}

	recuperada, err := repo.GetByOrderNo(ctx, orden.OrderNo)
	if err != nil {
		t.Fatalf("error recuperando orden: %v", err)
	}

	if recuperada.Status != domain.OrderQRRequested {
		t.Errorf("Status = %q, esperaba QR_REQUESTED", recuperada.Status)
	}
}

// TestPostgresOrderRepository_GetByPVSQrID verifica que se puede
// buscar una orden por el ID de QR de PVS.
func TestPostgresOrderRepository_GetByPVSQrID(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresOrderRepository(db)
	ctx := context.Background()

	orden := crearOrdenTest(t, repo, ctx)

	// Actualizar con QR ID
	err := repo.UpdateStatusAndFields(ctx, orden.OrderNo, domain.OrderQRShown,
		map[string]interface{}{"pvs_qr_id": "pvs-qr-test-123"})
	if err != nil {
		t.Fatalf("error actualizando QR: %v", err)
	}

	// Buscar por QR ID
	recuperada, err := repo.GetByPVSQrID(ctx, "pvs-qr-test-123")
	if err != nil {
		t.Fatalf("error buscando por QR ID: %v", err)
	}

	if recuperada.PvsQrID != "pvs-qr-test-123" {
		t.Errorf("PvsQrID = %q, esperaba pvs-qr-test-123", recuperada.PvsQrID)
	}
}

// TestPostgresOrderRepository_NotFound verifica que GetByOrderNo
// devuelve ErrOrderNotFound para ordenes inexistentes.
func TestPostgresOrderRepository_NotFound(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresOrderRepository(db)
	_, err := repo.GetByOrderNo(context.Background(), "no-existe")
	if err != domain.ErrOrderNotFound {
		t.Errorf("error = %v, esperaba ErrOrderNotFound", err)
	}
}

// TestPostgresIdempotencyStore_TryInsert verifica que TryInsert
// detecta correctamente claves nuevas y duplicadas.
func TestPostgresIdempotencyStore_TryInsert(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	store := NewPostgresIdempotencyStore(db)
	ctx := context.Background()

	// Primera insercion: debe devolver true
	inserted, err := store.TryInsert(ctx, "clave-unica-1")
	if err != nil {
		t.Fatalf("error en primera insercion: %v", err)
	}
	if !inserted {
		t.Error("primera insercion deberia ser true (nueva)")
	}

	// Segunda insercion con la misma clave: debe devolver false
	inserted, err = store.TryInsert(ctx, "clave-unica-1")
	if err != nil {
		t.Fatalf("error en segunda insercion: %v", err)
	}
	if inserted {
		t.Error("segunda insercion deberia ser false (duplicado)")
	}

	// Clave diferente: debe devolver true
	inserted, err = store.TryInsert(ctx, "clave-unica-2")
	if err != nil {
		t.Fatalf("error en tercera insercion: %v", err)
	}
	if !inserted {
		t.Error("tercera insercion (clave diferente) deberia ser true")
	}
}

// TestPostgresSyncLogRepo_InsertBestEffort verifica que Insert nunca
// devuelve error, incluso en condiciones normales.
func TestPostgresSyncLogRepo_InsertBestEffort(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresSyncLogRepo(db)
	ctx := context.Background()

	entry := &ports.SyncLogEntry{
		OrderNo:    "test-log-001",
		Vendor:     "PVS",
		Direction:  "outbound",
		Endpoint:   "/qr/pvs/service",
		Method:     "POST",
		StatusCode: 200,
		LatencyMs:  150,
	}
	err := repo.Insert(ctx, entry)
	if err != nil {
		t.Fatalf("error insertando sync log: %v", err)
	}
}

// TestPostgresRefundRepository_CreateEGetByRefundNo verifica que se puede
// crear un reembolso y recuperarlo por su refund_no.
func TestPostgresRefundRepository_CreateEGetByRefundNo(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresRefundRepository(db)
	ctx := context.Background()

	rf := &domain.Refund{
		RefundNo:   "test-refund-001",
		OrderNo:    "test-order-001",
		PriceCents: 15000,
		Motivo:     "test",
		Status:     domain.RefundPending,
	}

	err := repo.Create(ctx, rf)
	if err != nil {
		t.Fatalf("error creando reembolso: %v", err)
	}

	if rf.ID == 0 {
		t.Error("ID no fue asignado")
	}
	if rf.RequestedAt.IsZero() {
		t.Error("RequestedAt no fue asignado")
	}

	recuperado, err := repo.GetByRefundNo(ctx, rf.RefundNo)
	if err != nil {
		t.Fatalf("error recuperando reembolso: %v", err)
	}

	if recuperado.RefundNo != rf.RefundNo {
		t.Errorf("RefundNo = %q, esperaba %q", recuperado.RefundNo, rf.RefundNo)
	}
	if recuperado.Status != domain.RefundPending {
		t.Errorf("Status = %q, esperaba PENDING", recuperado.Status)
	}
}

// TestPostgresRefundRepository_UpdateStatus verifica que se puede
// actualizar el estado de un reembolso.
func TestPostgresRefundRepository_UpdateStatus(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresRefundRepository(db)
	ctx := context.Background()

	rf := &domain.Refund{
		RefundNo:   "test-refund-002",
		OrderNo:    "test-order-002",
		PriceCents: 10000,
		Status:     domain.RefundPending,
	}
	if err := repo.Create(ctx, rf); err != nil {
		t.Fatalf("error creando reembolso: %v", err)
	}

	err := repo.UpdateStatus(ctx, rf.RefundNo, domain.RefundSuccess)
	if err != nil {
		t.Fatalf("error actualizando estado: %v", err)
	}

	recuperado, err := repo.GetByRefundNo(ctx, rf.RefundNo)
	if err != nil {
		t.Fatalf("error recuperando reembolso: %v", err)
	}
	if recuperado.Status != domain.RefundSuccess {
		t.Errorf("Status = %q, esperaba SUCCESS", recuperado.Status)
	}
}

// TestPostgresRefundRepository_NotFound verifica que GetByRefundNo
// devuelve ErrRefundNotFound para reembolsos inexistentes.
func TestPostgresRefundRepository_NotFound(t *testing.T) {
	db := conectarDB(t)
	defer db.Close()

	repo := NewPostgresRefundRepository(db)
	_, err := repo.GetByRefundNo(context.Background(), "no-existe")
	if err != domain.ErrRefundNotFound {
		t.Errorf("error = %v, esperaba ErrRefundNotFound", err)
	}
}

// --- Servicio ---

// Helpers

// conectarDB se conecta a la base de datos de prueba.
// Salta el test si DATABASE_URL no esta configurada.
func conectarDB(t *testing.T) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL no configurada, saltando test de integracion")
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("error conectando a la base de datos: %v", err)
	}

	// Aplicar migraciones para el test
	migrar := []string{
		"../../migrations/001_orders.up.sql",
		"../../migrations/002_idempotency_keys.up.sql",
		"../../migrations/003_api_sync_log.up.sql",
		"../../migrations/004_refunds.up.sql",
	}
	for _, m := range migrar {
		sql, err := os.ReadFile(m)
		if err != nil {
			t.Skipf("no se pudo leer %s: %v", m, err)
		}
		if _, err := db.Exec(string(sql)); err != nil {
			t.Fatalf("error ejecutando %s: %v", m, err)
		}
	}

	return db
}

// crearOrdenTest es un helper que crea una orden de prueba en la DB.
func crearOrdenTest(t *testing.T, repo *PostgresOrderRepository, ctx context.Context) *domain.Order {
	t.Helper()

	orden := &domain.Order{
		OrderNo:    "test-" + time.Now().Format("150405.000"),
		DeviceID:   "dev-test",
		ObjectID:   "drink-test",
		PriceCents: 10000,
		Status:     domain.OrderReceived,
	}

	if err := repo.Create(ctx, orden); err != nil {
		t.Fatalf("error creando orden de test: %v", err)
	}
	return orden
}
