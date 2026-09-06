package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"redstone/download"
	"redstone/manifest"
	"redstone/mirrors"
	"redstone/modes"
	"strings"
)

type objectEntry struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type assetIndex struct {
	Objects        map[string]objectEntry `json:"objects"`
	MapToResources bool                   `json:"map_to_resources"`
	Virtual        bool                   `json:"virtual"`
}

func IsPreAssetsEra(assetsID string) bool {
	return assetsID == "" || assetsID == "pre-1.6"
}

func IsLegacyVirtual(assetsID string) bool {
	return assetsID == "legacy"
}

type Manager struct {
	GameDir string
	Mirrors mirrors.Config
	Online  bool
	Mode    modes.AssetMode
}

func (m *Manager) EnsureIndex(ctx context.Context, ref *manifest.AssetIndexRef) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("у версии нет assetIndex (вероятно, это версия эпохи pre-1.6)")
	}
	dest := filepath.Join(m.GameDir, "assets", "indexes", ref.ID+".json")

	if m.Online && ref.URL != "" {
		chain := m.Mirrors.BuildChain(mirrors.KindAsset, ref.URL)
		errs := download.Pool(ctx, []download.Task{{
			ID: "assetIndex", URLs: chain, Dest: dest, SHA1: ref.SHA1, Size: ref.Size,
		}}, download.Options{Workers: 1}, nil)
		if len(errs) > 0 && !fileExists(dest) {
			return "", fmt.Errorf("не удалось скачать индекс ассетов: %v", errs[0])
		}
	}

	if !fileExists(dest) {
		return "", fmt.Errorf("индекс ассетов %s отсутствует локально и нет сети", ref.ID)
	}
	return dest, nil
}

func matchesAssetMode(virtualPath string, mode modes.AssetMode) bool {
	lower := strings.ToLower(virtualPath)
	switch mode {
	case modes.AssetModeNone:
		return false
	case modes.AssetModeSounds:
		return strings.Contains(lower, "sounds/") || strings.HasSuffix(lower, ".ogg")
	case modes.AssetModeTextures:
		return strings.Contains(lower, "textures/") || strings.HasSuffix(lower, ".png")
	case modes.AssetModeMinimal:
		return strings.Contains(lower, "lang/") || strings.Contains(lower, "textures/") || strings.Contains(lower, "icons/")
	case modes.AssetModeAll, "":
		fallthrough
	default:
		return true
	}
}

func (m *Manager) EnsureObjects(ctx context.Context, indexPath, assetsID string, progress chan<- download.Progress) error {
	if m.Mode == modes.AssetModeNone {
		return nil
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	var idx assetIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("повреждён индекс ассетов: %w", err)
	}

	objectsDir := filepath.Join(m.GameDir, "assets", "objects")
	var tasks []download.Task

	for virtualPath, obj := range idx.Objects {
		if len(obj.Hash) < 2 {
			continue
		}
		if !matchesAssetMode(virtualPath, m.Mode) {
			continue
		}
		prefix := obj.Hash[:2]
		realDest := filepath.Join(objectsDir, prefix, obj.Hash)
		originalURL := fmt.Sprintf("https://resources.download.minecraft.net/%s/%s", prefix, obj.Hash)
		chain := m.Mirrors.BuildChain(mirrors.KindAsset, originalURL)

		tasks = append(tasks, download.Task{
			ID: virtualPath, URLs: chain, Dest: realDest, SHA1: obj.Hash, Size: obj.Size,
		})

		if IsLegacyVirtual(assetsID) || idx.MapToResources {
			legacyDest := filepath.Join(m.GameDir, "assets", "virtual", "legacy", filepath.FromSlash(virtualPath))
			tasks = append(tasks, download.Task{
				ID: virtualPath + "#legacy", URLs: chain, Dest: legacyDest, SHA1: obj.Hash, Size: obj.Size,
			})
		}
	}

	if !m.Online {
		download.Pool(ctx, tasks, download.Options{Workers: 8}, progress)
		return nil
	}

	errs := download.Pool(ctx, tasks, download.Options{Workers: 8, Retries: 3}, progress)
	if len(errs) > 0 {
		return fmt.Errorf("часть ассетов не скачалась (%d ошибок), первая: %v", len(errs), errs[0])
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
