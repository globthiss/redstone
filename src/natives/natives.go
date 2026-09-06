package natives

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"redstone/libraries"
	"redstone/manifest"
)

type Extractor struct {
	Dir string // путь к временной папке (заполняется в CreateTempDir)
}

func (e *Extractor) CreateTempDir() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "redstone_natives_"+hex.EncodeToString(buf))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	e.Dir = dir
	return dir, nil
}

func nativeFileExt(osName string) []string {
	switch osName {
	case "windows":
		return []string{".dll"}
	case "mac":
		return []string{".dylib", ".jnilib"}
	default:
		return []string{".so"}
	}
}

func isExcluded(path string, excludes []string) bool {
	for _, ex := range excludes {
		ex = strings.TrimSuffix(ex, "/")
		if strings.HasPrefix(path, ex) {
			return true
		}
	}
	return false
}

func (e *Extractor) ExtractAll(resolvedNatives []libraries.Resolved, libs []manifest.Library, osName string) error {
	exts := nativeFileExt(osName)
	excludeByName := map[string][]string{}
	for _, l := range libs {
		if l.Extract != nil {
			excludeByName[l.Name] = l.Extract.Exclude
		}
	}

	for _, native := range resolvedNatives {
		baseName := strings.TrimSuffix(native.Name, "-natives")
		excludes := excludeByName[baseName]

		if err := extractJar(native.Path, e.Dir, exts, excludes); err != nil {
			return fmt.Errorf("не удалось распаковать нативы из %s: %w", native.Path, err)
		}
	}
	return nil
}

func extractJar(jarPath, destDir string, exts []string, excludes []string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if isExcluded(f.Name, excludes) {
			continue
		}
		matched := false
		for _, ext := range exts {
			if strings.HasSuffix(strings.ToLower(f.Name), ext) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		outPath := filepath.Join(destDir, filepath.Base(f.Name))
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (e *Extractor) Cleanup() error {
	if e.Dir == "" {
		return nil
	}
	return os.RemoveAll(e.Dir)
}
