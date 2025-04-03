// internal/util/tempdir.go
package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// SetupHostRunDir creates a unique temporary directory for a run on the host.
// It returns the path to the created directory and a cleanup function.
func SetupHostRunDir(baseDir string) (runDir string, cleanup func(), err error) {
	runID := uuid.New().String()
	runDir = filepath.Join(baseDir, runID)

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create base temp directory %s: %w", baseDir, err)
	}

	// Create the specific run directory
	if err := os.Mkdir(runDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create host run temp dir %s: %w", runDir, err)
	}
	log.Printf("Created host temp dir: %s", runDir)

	cleanup = func() {
		if err := os.RemoveAll(runDir); err != nil {
			log.Printf("Warning: failed to clean up host temp dir %s: %v", runDir, err)
		} else {
			log.Printf("Cleaned up host temp dir: %s", runDir)
		}
	}

	return runDir, cleanup, nil
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dirName string) error {
    err := os.MkdirAll(dirName, 0755)
    if err != nil {
        return fmt.Errorf("failed to create directory %s: %w", dirName, err)
    }
    return nil
}