package domain_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDomainHasNoInfrastructureOrTransportDependencies validates that all packages
// in internal/domain only import standard library or other pure domain packages.
func TestDomainHasNoInfrastructureOrTransportDependencies(t *testing.T) {
	domainRoot := "."
	forbiddenPrefixes := []string{
		"net/http",
		"google.golang.org/grpc",
		"github.com/jackc/pgx",
		"github.com/go-chi",
		"github.com/aurora-vm/aurora/internal/infra",
		"github.com/aurora-vm/aurora/internal/transport",
		"github.com/aurora-vm/aurora/internal/app",
	}

	err := filepath.Walk(domainRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, parseErr, "failed to parse domain file: %s", path)

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range forbiddenPrefixes {
				assert.False(t, strings.HasPrefix(importPath, forbidden),
					"Clean Architecture Violation in %s: domain package must not import %s", path, importPath)
			}
		}
		return nil
	})

	require.NoError(t, err)
}
