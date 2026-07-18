package util

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestTempDirectoryLogsNeverContainPath(t *testing.T) {
	const sentinelPath = "SENTINEL_TEMP_PATH_1eb777"
	for _, debug := range []bool{false, true} {
		debug := debug
		t.Run(map[bool]string{false: "default", true: "debug"}[debug], func(t *testing.T) {
			var logs bytes.Buffer
			oldWriter := log.Writer()
			oldFlags := log.Flags()
			oldDebug := DebugMode
			log.SetOutput(&logs)
			log.SetFlags(0)
			if debug {
				t.Setenv("CROJ_DEBUG", "1")
			} else {
				t.Setenv("CROJ_DEBUG", "0")
			}
			InitDebugMode()
			t.Cleanup(func() {
				log.SetOutput(oldWriter)
				log.SetFlags(oldFlags)
				DebugMode = oldDebug
			})

			_, cleanup, err := SetupHostRunDir(filepath.Join(t.TempDir(), sentinelPath))
			if err != nil {
				t.Fatalf("SetupHostRunDir: %v", err)
			}
			cleanup()
			if strings.Contains(logs.String(), sentinelPath) {
				t.Fatalf("temp-directory logs contain host path:\n%s", logs.String())
			}
		})
	}
}
