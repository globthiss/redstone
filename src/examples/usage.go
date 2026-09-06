//go:build ignore

package examples

import (
	"context"
	"path/filepath"

	"redstone/authutil"
	"redstone/core"
	"redstone/download"
	"redstone/modes"
)

func LaunchOffline(gameDir, username, versionID string) {
	events := make(chan core.StageEvent, 32)
	progress := make(chan download.Progress, 256)

	go func() {
		for range events {
		}
	}()
	go func() {
		for range progress {
		}
	}()

	cfg := modes.Default()
	cfg.Java = modes.JavaModeSmart
	cfg.Memory = modes.MemoryModeAuto
	cfg.Window = modes.WindowModeWindowed

	session := authutil.Session{Type: authutil.AuthOffline, Username: username}

	launched, err := core.Launch(context.Background(), core.LaunchOptions{
		GameDir:   gameDir,
		VersionID: versionID,
		Session:   session,
		Modes:     cfg,
		Events:    events,
		Progress:  progress,
	})
	if err != nil {
		panic(err)
	}

	for line := range launched.Process.Logs {
		_ = line
	}
	launched.Process.Wait()
}

func LaunchMicrosoft(gameDir, versionID, msClientID, accountKey string) {
}

func customGameDir() string {
	return filepath.Join("D:", "MyLauncher", "gamedata")
}
