package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"redstone/download"
	"redstone/mirrors"
)

const versionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type VersionManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []VersionEntry `json:"versions"`
}

type VersionEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // release, snapshot, old_beta, old_alpha
	URL         string `json:"url"`
	Time        string `json:"time"`
	ReleaseTime string `json:"releaseTime"`
	SHA1        string `json:"sha1"`
}

type DownloadArtifact struct {
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type Rule struct {
	Action string `json:"action"` // allow / disallow
	OS     *struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Arch    string `json:"arch"`
	} `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

type LibraryDownloads struct {
	Artifact    *DownloadArtifact           `json:"artifact,omitempty"`
	Classifiers map[string]DownloadArtifact `json:"classifiers,omitempty"`
}

type Library struct {
	Name      string            `json:"name"`
	Downloads *LibraryDownloads `json:"downloads,omitempty"`
	Rules     []Rule            `json:"rules,omitempty"`
	Natives   map[string]string `json:"natives,omitempty"` // старый формат: {"windows":"natives-windows"}
	Extract   *struct {
		Exclude []string `json:"exclude"`
	} `json:"extract,omitempty"`
	URL string `json:"url,omitempty"`
}

type ArgumentValue struct {
	Plain string
	Rules []Rule
	Value []string // строка или список строк
}

func (a *ArgumentValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		a.Plain = s
		a.Value = []string{s}
		return nil
	}
	var obj struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	a.Rules = obj.Rules
	var single string
	if err := json.Unmarshal(obj.Value, &single); err == nil {
		a.Value = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(obj.Value, &multi); err == nil {
		a.Value = multi
		return nil
	}
	return fmt.Errorf("неизвестный формат аргумента: %s", string(b))
}

type Arguments struct {
	Game []ArgumentValue `json:"game,omitempty"`
	JVM  []ArgumentValue `json:"jvm,omitempty"`
}

type AssetIndexRef struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	TotalSize int64  `json:"totalSize"`
}

type JavaVersionRef struct {
	Component    string `json:"component"`    // напр. "java-runtime-gamma"
	MajorVersion int    `json:"majorVersion"` // напр. 17
}

type Version struct {
	ID         string                      `json:"id"`
	Type       string                      `json:"type"`
	MainClass  string                      `json:"mainClass"`
	Assets     string                      `json:"assets"`              // id индекса ассетов, "legacy"/"pre-1.6" для старых версий
	Downloads  map[string]DownloadArtifact `json:"downloads,omitempty"` // "client","server"
	Libraries  []Library                   `json:"libraries"`
	AssetIndex *AssetIndexRef              `json:"assetIndex,omitempty"`
	JavaVer    *JavaVersionRef             `json:"javaVersion,omitempty"`

	Arguments          *Arguments `json:"arguments,omitempty"`
	MinecraftArguments string     `json:"minecraftArguments,omitempty"`

	InheritsFrom string `json:"inheritsFrom,omitempty"`

	Logging *struct {
		Client struct {
			Argument string           `json:"argument"`
			File     DownloadArtifact `json:"file"`
		} `json:"client"`
	} `json:"logging,omitempty"`

	MinimumLauncherVersion int `json:"minimumLauncherVersion,omitempty"`
}

func (v *Version) IsLegacyArguments() bool {
	return v.Arguments == nil && v.MinecraftArguments != ""
}

func (v *Version) RequiredJavaMajor() int {
	if v.JavaVer != nil && v.JavaVer.MajorVersion > 0 {
		return v.JavaVer.MajorVersion
	}
	return 8
}

type Manager struct {
	GameDir string
	Mirrors mirrors.Config
	Online  bool
}

func (m *Manager) FetchVersionManifest(ctx context.Context) (*VersionManifest, error) {
	cachePath := filepath.Join(m.GameDir, "versions", "version_manifest_v2.json")

	if m.Online {
		chain := m.Mirrors.BuildChain(mirrors.KindVersionManifest, versionManifestURL)
		errs := download.Pool(ctx, []download.Task{{
			ID: "version_manifest", URLs: chain, Dest: cachePath,
		}}, download.Options{Workers: 1}, nil)
		if len(errs) > 0 && !fileExists(cachePath) {
			return nil, fmt.Errorf("не удалось получить манифест версий: %v", errs[0])
		}
	}

	if !fileExists(cachePath) {
		return nil, fmt.Errorf("манифест версий отсутствует локально и нет сети (офлайн-режим)")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var vm VersionManifest
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("повреждён кэш манифеста версий: %w", err)
	}
	return &vm, nil
}

func (m *Manager) FetchVersion(ctx context.Context, versionID, entryURL, entrySHA1 string) (*Version, error) {
	dir := filepath.Join(m.GameDir, "versions", versionID)
	path := filepath.Join(dir, versionID+".json")

	if m.Online && entryURL != "" {
		chain := m.Mirrors.BuildChain(mirrors.KindVersionJSON, entryURL)
		errs := download.Pool(ctx, []download.Task{{
			ID: versionID + ".json", URLs: chain, Dest: path, SHA1: entrySHA1,
		}}, download.Options{Workers: 1}, nil)
		if len(errs) > 0 && !fileExists(path) {
			return nil, fmt.Errorf("не удалось получить version.json для %s: %v", versionID, errs[0])
		}
	}

	if !fileExists(path) {
		return nil, fmt.Errorf("version.json для %s отсутствует локально и нет сети", versionID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v Version
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("повреждён version.json %s: %w", versionID, err)
	}
	if v.ID == "" {
		v.ID = versionID
	}

	if v.Downloads == nil {
		v.Downloads = map[string]DownloadArtifact{}
	}

	return &v, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
