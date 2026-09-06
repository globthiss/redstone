package libraries

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"redstone/download"
	"redstone/javart"
	"redstone/manifest"
	"redstone/mirrors"
	"redstone/modes"
)

type Resolved struct {
	Name       string
	Path       string // абсолютный путь к jar в libraries/
	IsNative   bool
	NativePath string // путь к natives-jar (если есть отдельный classifier)
}

func currentOSRuleName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	default:
		return "linux"
	}
}

func EvaluateRules(rules []manifest.Rule, plat javart.Platform) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		match := true
		if r.OS != nil {
			if r.OS.Name != "" && r.OS.Name != currentOSRuleName() {
				match = false
			}
			if r.OS.Arch != "" && !archMatches(r.OS.Arch, plat) {
				match = false
			}
		}
		if !match {
			continue
		}
		allowed = r.Action == "allow"
	}
	return allowed
}

func archMatches(ruleArch string, plat javart.Platform) bool {
	switch ruleArch {
	case "x86":
		return plat.Arch == "x86"
	case "x86_64", "amd64":
		return plat.Arch == "x64"
	default:
		return true
	}
}

func nativeClassifier(lib manifest.Library, plat javart.Platform) string {
	if lib.Natives == nil {
		return ""
	}
	osKey := map[string]string{"windows": "windows", "mac": "osx", "linux": "linux"}[plat.OS]
	tmpl, ok := lib.Natives[osKey]
	if !ok {
		return ""
	}
	arch := "32"
	if plat.Arch == "x64" || plat.Arch == "arm64" {
		arch = "64"
	}
	return strings.ReplaceAll(tmpl, "${arch}", arch)
}

func Filter(libs []manifest.Library, plat javart.Platform) []manifest.Library {
	out := make([]manifest.Library, 0, len(libs))
	for _, l := range libs {
		if EvaluateRules(l.Rules, plat) {
			out = append(out, l)
		}
	}
	return out
}

func FilterByMode(libs []manifest.Library, plat javart.Platform, mode modes.LibraryMode) []manifest.Library {
	base := Filter(libs, plat)
	switch mode {
	case modes.LibraryModeNative:
		out := make([]manifest.Library, 0, len(base))
		for _, l := range base {
			if l.Natives != nil || (l.Downloads != nil && len(l.Downloads.Classifiers) > 0) {
				out = append(out, l)
			}
		}
		return out
	case modes.LibraryModeMinimal, modes.LibraryModeCurrentOS, modes.LibraryModeAll:
		fallthrough
	default:
		return base
	}
}

func mavenPathFromName(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return strings.ReplaceAll(name, ":", "/") + ".jar"
	}
	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact := parts[1]
	version := parts[2]
	classifier := ""
	if len(parts) > 3 {
		classifier = "-" + parts[3]
	}
	fileName := fmt.Sprintf("%s-%s%s.jar", artifact, version, classifier)
	return filepath.Join(group, artifact, version, fileName)
}

func Resolve(libs []manifest.Library, librariesDir string, plat javart.Platform, mirrorCfg mirrors.Config) ([]Resolved, []download.Task) {
	var resolved []Resolved
	var tasks []download.Task

	for _, lib := range libs {
		if lib.Downloads != nil && lib.Downloads.Artifact != nil && lib.Downloads.Artifact.URL != "" {
			art := lib.Downloads.Artifact
			relPath := mavenPathFromName(lib.Name)
			dest := filepath.Join(librariesDir, filepath.FromSlash(relPath))
			resolved = append(resolved, Resolved{Name: lib.Name, Path: dest})
			tasks = append(tasks, download.Task{
				ID: lib.Name, URLs: mirrorCfg.BuildChain(mirrors.KindLibrary, art.URL),
				Dest: dest, SHA1: art.SHA1, Size: art.Size,
			})
		} else if lib.Downloads == nil && lib.URL != "" {
			relPath := mavenPathFromName(lib.Name)
			dest := filepath.Join(librariesDir, filepath.FromSlash(relPath))
			fullURL := strings.TrimRight(lib.URL, "/") + "/" + filepath.ToSlash(relPath)
			resolved = append(resolved, Resolved{Name: lib.Name, Path: dest})
			tasks = append(tasks, download.Task{
				ID: lib.Name, URLs: mirrorCfg.BuildChain(mirrors.KindLibrary, fullURL), Dest: dest,
			})
		}

		classifierKey := nativeClassifier(lib, plat)
		if classifierKey != "" && lib.Downloads != nil {
			if art, ok := lib.Downloads.Classifiers[classifierKey]; ok && art.URL != "" {
				relPath := mavenPathFromName(lib.Name + ":" + classifierKey)
				dest := filepath.Join(librariesDir, filepath.FromSlash(relPath))
				resolved = append(resolved, Resolved{Name: lib.Name + "-natives", Path: dest, IsNative: true})
				tasks = append(tasks, download.Task{
					ID: lib.Name + "-natives", URLs: mirrorCfg.BuildChain(mirrors.KindLibrary, art.URL),
					Dest: dest, SHA1: art.SHA1, Size: art.Size,
				})
			}
		}
	}

	return resolved, tasks
}

func Download(ctx context.Context, tasks []download.Task, mode modes.DownloadMode, progress chan<- download.Progress) []error {
	return download.Pool(ctx, tasks, download.Options{Workers: 8, Retries: 3, Mode: mode}, progress)
}

func ClassPath(resolved []Resolved, clientJarPath string, plat javart.Platform) string {
	sep := ":"
	if plat.OS == "windows" {
		sep = ";"
	}
	parts := make([]string, 0, len(resolved)+1)
	seen := map[string]bool{}
	for _, r := range resolved {
		if r.IsNative {
			continue
		}
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		parts = append(parts, r.Path)
	}
	parts = append(parts, clientJarPath)
	return strings.Join(parts, sep)
}
