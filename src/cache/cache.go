package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"redstone/modes"
)

type Manager struct {
	GameDir string
}

func New(gameDir string) *Manager {
	return &Manager{GameDir: gameDir}
}

func (m *Manager) Apply(mode modes.CacheMode) error {
	switch mode {
	case modes.CacheModeNone:
		return m.wipe(filepath.Join(m.GameDir, "libraries"), filepath.Join(m.GameDir, "assets", "objects"))
	case modes.CacheModeMetadata:
		return m.wipe(filepath.Join(m.GameDir, "libraries"), filepath.Join(m.GameDir, "assets", "objects"))
	case modes.CacheModeSmart:
		return m.cleanOrphans()
	case modes.CacheModeCore, modes.CacheModeAll:
		return nil
	default:
		return nil
	}
}

func (m *Manager) wipe(dirs ...string) error {
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) cleanOrphans() error {
	referenced := map[string]bool{}

	versionsDir := filepath.Join(m.GameDir, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jsonPath := filepath.Join(versionsDir, e.Name(), e.Name()+".json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			continue
		}
		var v struct {
			Libraries []struct {
				Downloads struct {
					Artifact struct {
						Path string `json:"path"`
					} `json:"artifact"`
				} `json:"downloads"`
			} `json:"libraries"`
			AssetIndex struct {
				ID string `json:"id"`
			} `json:"assetIndex"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		for _, lib := range v.Libraries {
			if lib.Downloads.Artifact.Path != "" {
				referenced[filepath.Join(m.GameDir, "libraries", filepath.FromSlash(lib.Downloads.Artifact.Path))] = true
			}
		}
		if v.AssetIndex.ID != "" {
			referenced[filepath.Join(m.GameDir, "assets", "indexes", v.AssetIndex.ID+".json")] = true
			idxData, err := os.ReadFile(filepath.Join(m.GameDir, "assets", "indexes", v.AssetIndex.ID+".json"))
			if err == nil {
				var idx struct {
					Objects map[string]struct {
						Hash string `json:"hash"`
					} `json:"objects"`
				}
				if json.Unmarshal(idxData, &idx) == nil {
					for _, obj := range idx.Objects {
						if len(obj.Hash) >= 2 {
							referenced[filepath.Join(m.GameDir, "assets", "objects", obj.Hash[:2], obj.Hash)] = true
						}
					}
				}
			}
		}
	}

	var removed int
	filepath.Walk(filepath.Join(m.GameDir, "libraries"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !referenced[path] && strings.HasSuffix(path, ".jar") {
			os.Remove(path)
			removed++
		}
		return nil
	})
	filepath.Walk(filepath.Join(m.GameDir, "assets", "objects"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !referenced[path] {
			os.Remove(path)
			removed++
		}
		return nil
	})

	return nil
}
