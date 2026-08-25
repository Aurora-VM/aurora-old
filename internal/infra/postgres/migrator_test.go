package postgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrator_LoadMigrations(t *testing.T) {
	tempDir := t.TempDir()

	upSQL := "CREATE TABLE test (id INT);"
	downSQL := "DROP TABLE test;"

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "000001_init.up.sql"), []byte(upSQL), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "000001_init.down.sql"), []byte(downSQL), 0644))

	migrator := NewMigrator(nil, tempDir)
	migrations, err := migrator.LoadMigrations()
	require.NoError(t, err)
	require.Len(t, migrations, 1)

	assert.Equal(t, 1, migrations[0].Version)
	assert.Equal(t, upSQL, migrations[0].UpSQL)
	assert.Equal(t, downSQL, migrations[0].DownSQL)
}

func TestProjectMigrations_ValidFiles(t *testing.T) {
	// Tests the actual migrations directory of the project
	migrator := NewMigrator(nil, "../../../migrations")
	migrations, err := migrator.LoadMigrations()
	require.NoError(t, err)
	assert.NotEmpty(t, migrations, "Should have at least initial migration")
	assert.Equal(t, 1, migrations[0].Version)
	assert.Contains(t, migrations[0].UpSQL, "CREATE TABLE IF NOT EXISTS users")
	assert.Contains(t, migrations[0].DownSQL, "DROP TABLE IF EXISTS users")
}
