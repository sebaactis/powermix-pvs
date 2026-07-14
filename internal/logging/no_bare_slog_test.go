package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoBareSlogInMigratedPackages verifica que los paquetes migrados a
// logging.From(ctx) no contengan llamadas bare a slog.Info|Error|Debug|Warn.
//
// Paquetes exentos: cmd/server/ (entry point), internal/logging/ (este paquete).
// También se excluyen archivos _test.go.
func TestNoBareSlogInMigratedPackages(t *testing.T) {
	packages := []string{
		"internal/handler",
		"internal/service",
		"internal/client/pvs",
		"internal/client/gs",
		"internal/store",
		"internal/reconciler",
	}

	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("no se pudo encontrar la raíz del proyecto: %v", err)
	}

	bareSlogRE := regexp.MustCompile(`\bslog\.(Info|Error|Debug|Warn)\(`)

	for _, pkg := range packages {
		pkgPath := filepath.Join(root, pkg)
		entries, err := os.ReadDir(pkgPath)
		if err != nil {
			t.Fatalf("no se pudo leer el paquete %s: %v", pkg, err)
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}

			content, err := os.ReadFile(filepath.Join(pkgPath, name))
			if err != nil {
				t.Fatalf("no se pudo leer %s/%s: %v", pkg, name, err)
			}

			lines := strings.Split(string(content), "\n")
			for i, line := range lines {
				if bareSlogRE.MatchString(line) {
					t.Errorf("bare slog call encontrado en %s/%s:%d: %s",
						pkg, name, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// findProjectRoot sube desde el directorio de este archivo hasta encontrar go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
