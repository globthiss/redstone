package javart

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"redstone/download"
	"redstone/modes"
)

type Platform struct {
	OS   string // "windows", "linux", "mac"
	Arch string // "x64", "x86", "arm64"
}

func CurrentPlatform() Platform {
	p := Platform{}
	switch runtime.GOOS {
	case "windows":
		p.OS = "windows"
	case "darwin":
		p.OS = "mac"
	default:
		p.OS = "linux"
	}
	switch runtime.GOARCH {
	case "amd64":
		p.Arch = "x64"
	case "386":
		p.Arch = "x86"
	case "arm64":
		p.Arch = "arm64"
	case "arm":
		p.Arch = "arm"
	default:
		p.Arch = runtime.GOARCH
	}
	return p
}

func (p Platform) mojangRuntimeKey() string {
	switch {
	case p.OS == "windows" && p.Arch == "x64":
		return "windows-x64"
	case p.OS == "windows" && p.Arch == "x86":
		return "windows-x86"
	case p.OS == "windows" && p.Arch == "arm64":
		return "windows-arm64"
	case p.OS == "mac" && p.Arch == "x64":
		return "mac-os"
	case p.OS == "mac" && p.Arch == "arm64":
		return "mac-os-arm64"
	case p.OS == "linux" && p.Arch == "x64":
		return "linux"
	case p.OS == "linux" && p.Arch == "x86":
		return "linux-i386"
	default:
		return "linux"
	}
}

type Manager struct {
	RuntimesDir string
	Online      bool
	HTTPClient  *http.Client
	Mode        modes.JavaMode
	CustomPath  string
	EmbeddedDir string
}

type Resolved struct {
	JavaBinary string
	HomeDir    string
	Major      int
	Source     modes.JavaSource
}

func javaExeName(p Platform) string {
	if p.OS == "windows" {
		return "java.exe"
	}
	return "java"
}

func (m *Manager) Resolve(ctx context.Context, major int, plat Platform) (*Resolved, error) {
	switch m.Mode {
	case modes.JavaModeSystem:
		return m.resolveSystem(major, plat)
	case modes.JavaModeEnv:
		return m.resolveEnv(major, plat)
	case modes.JavaModeCustom, modes.JavaModeManual:
		return m.resolveCustom(major, plat)
	case modes.JavaModeEmbedded:
		return m.resolveEmbedded(major, plat)
	case modes.JavaModeSmart:
		if r, err := m.resolveSystem(major, plat); err == nil {
			return r, nil
		}
		if r, err := m.resolveCustom(major, plat); err == nil {
			return r, nil
		}
		if r, err := m.resolveEnv(major, plat); err == nil {
			return r, nil
		}
		return m.resolveAuto(ctx, major, plat)
	case modes.JavaModeAuto, "":
		fallthrough
	default:
		return m.resolveAuto(ctx, major, plat)
	}
}

func (m *Manager) resolveCustom(major int, plat Platform) (*Resolved, error) {
	if m.CustomPath == "" {
		return nil, fmt.Errorf("кастомный путь Java не задан (Manager.CustomPath)")
	}
	if !fileRunnable(m.CustomPath) {
		return nil, fmt.Errorf("java по указанному пути не найдена: %s", m.CustomPath)
	}
	gotMajor, err := detectJavaMajor(m.CustomPath)
	if err == nil && gotMajor != major {
		return nil, fmt.Errorf("java по пути %s имеет версию %d, требуется %d", m.CustomPath, gotMajor, major)
	}
	return &Resolved{JavaBinary: m.CustomPath, HomeDir: filepath.Dir(filepath.Dir(m.CustomPath)), Major: major, Source: modes.JavaSourceCustom}, nil
}

func (m *Manager) resolveEnv(major int, plat Platform) (*Resolved, error) {
	home := os.Getenv("JAVA_HOME")
	if home == "" {
		return nil, fmt.Errorf("переменная окружения JAVA_HOME не установлена")
	}
	bin := filepath.Join(home, "bin", javaExeName(plat))
	if !fileRunnable(bin) {
		return nil, fmt.Errorf("java не найдена в JAVA_HOME: %s", bin)
	}
	gotMajor, err := detectJavaMajor(bin)
	if err == nil && gotMajor != major {
		return nil, fmt.Errorf("java из JAVA_HOME имеет версию %d, требуется %d", gotMajor, major)
	}
	return &Resolved{JavaBinary: bin, HomeDir: home, Major: major, Source: modes.JavaSourceEnv}, nil
}

func (m *Manager) resolveSystem(major int, plat Platform) (*Resolved, error) {
	name := "java"
	if plat.OS == "windows" {
		name = "java.exe"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("java не найдена в PATH")
	}
	gotMajor, err := detectJavaMajor(path)
	if err == nil && gotMajor != major {
		return nil, fmt.Errorf("системная java имеет версию %d, требуется %d", gotMajor, major)
	}
	return &Resolved{JavaBinary: path, HomeDir: filepath.Dir(filepath.Dir(path)), Major: major, Source: modes.JavaSourceSystem}, nil
}

func (m *Manager) resolveEmbedded(major int, plat Platform) (*Resolved, error) {
	if m.EmbeddedDir == "" {
		return nil, fmt.Errorf("путь встроенной java не задан (Manager.EmbeddedDir)")
	}
	bin := javaBinPath(m.EmbeddedDir, plat)
	if !fileRunnable(bin) {
		return nil, fmt.Errorf("встроенная java не найдена: %s", bin)
	}
	return &Resolved{JavaBinary: bin, HomeDir: m.EmbeddedDir, Major: major, Source: modes.JavaSourceEmbedded}, nil
}

func detectJavaMajor(javaBin string) (int, error) {
	out, err := exec.Command(javaBin, "-version").CombinedOutput()
	if err != nil {
		return 0, err
	}
	text := string(out)
	re := regexp.MustCompile(`version "(\d+)(\.(\d+))?`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, fmt.Errorf("не удалось распознать версию java из вывода: %s", text)
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, err
	}
	if major == 1 && len(match) >= 4 && match[3] != "" {
		major, err = strconv.Atoi(match[3])
		if err != nil {
			return 0, err
		}
	}
	return major, nil
}

func (m *Manager) resolveAuto(ctx context.Context, major int, plat Platform) (*Resolved, error) {
	verDir := filepath.Join(m.RuntimesDir, fmt.Sprintf("java-%d-%s-%s", major, plat.OS, plat.Arch))
	binPath := javaBinPath(verDir, plat)

	if fileRunnable(binPath) {
		return &Resolved{JavaBinary: binPath, HomeDir: verDir, Major: major, Source: modes.JavaSourceCache}, nil
	}

	if !m.Online {
		return nil, fmt.Errorf("java %d не найдена локально (%s) и нет сети для загрузки", major, binPath)
	}

	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	if archiveURL, sha1 := tryMojang(ctx, client, major, plat); archiveURL != "" {
		if err := downloadAndExtract(ctx, archiveURL, sha1, verDir, plat); err == nil {
			if fileRunnable(binPath) {
				return &Resolved{JavaBinary: binPath, HomeDir: verDir, Major: major, Source: modes.JavaSourceAuto}, nil
			}
		}
	}

	if archiveURL := tryAdoptium(ctx, client, major, plat); archiveURL != "" {
		if err := downloadAndExtract(ctx, archiveURL, "", verDir, plat); err == nil {
			if fileRunnable(binPath) {
				return &Resolved{JavaBinary: binPath, HomeDir: verDir, Major: major, Source: modes.JavaSourceAuto}, nil
			}
		}
	}

	if archiveURL := tryCorretto(major, plat); archiveURL != "" {
		if err := downloadAndExtract(ctx, archiveURL, "", verDir, plat); err == nil {
			if fileRunnable(binPath) {
				return &Resolved{JavaBinary: binPath, HomeDir: verDir, Major: major, Source: modes.JavaSourceAuto}, nil
			}
		}
	}

	if archiveURL := tryZulu(ctx, client, major, plat); archiveURL != "" {
		if err := downloadAndExtract(ctx, archiveURL, "", verDir, plat); err == nil {
			if fileRunnable(binPath) {
				return &Resolved{JavaBinary: binPath, HomeDir: verDir, Major: major, Source: modes.JavaSourceAuto}, nil
			}
		}
	}

	return nil, fmt.Errorf("не удалось получить Java %d ни у одного из провайдеров (Mojang/Adoptium/Corretto/Zulu)", major)
}

func javaBinPath(homeDir string, plat Platform) string {
	name := javaExeName(plat)
	candidates := []string{
		filepath.Join(homeDir, "bin", name),
		filepath.Join(homeDir, "Contents", "Home", "bin", name),
	}
	for _, c := range candidates {
		if fileRunnable(c) {
			return c
		}
	}
	return candidates[0]
}

func fileRunnable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return true
}

type mojangRuntimeManifest map[string]map[string][]struct {
	Availability struct {
		Group    int `json:"group"`
		Progress int `json:"progress"`
	} `json:"availability"`
	Manifest struct {
		SHA1 string `json:"sha1"`
		Size int64  `json:"size"`
		URL  string `json:"url"`
	} `json:"manifest"`
	Version struct {
		Name string `json:"name"`
	} `json:"version"`
}

func tryMojang(ctx context.Context, client *http.Client, major int, plat Platform) (archiveURL, sha1 string) {
	const indexURL = "https://piston-meta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"
	body, err := httpGet(ctx, client, indexURL)
	if err != nil {
		return "", ""
	}
	var all mojangRuntimeManifest
	if err := json.Unmarshal(body, &all); err != nil {
		return "", ""
	}
	platEntries, ok := all[plat.mojangRuntimeKey()]
	if !ok {
		return "", ""
	}
	component := mojangComponentFor(major)
	entries, ok := platEntries[component]
	if !ok || len(entries) == 0 {
		return "", ""
	}
	return entries[0].Manifest.URL, entries[0].Manifest.SHA1
}

func mojangComponentFor(major int) string {
	switch {
	case major <= 8:
		return "jre-legacy"
	case major <= 16:
		return "java-runtime-alpha"
	case major == 17:
		return "java-runtime-gamma"
	default:
		return "java-runtime-delta"
	}
}

func downloadAndExtract(ctx context.Context, url, sha1, destDir string, plat Platform) error {
	if strings.HasSuffix(url, ".json") || strings.Contains(url, "piston-meta") {
		return installMojangFiles(ctx, url, sha1, destDir)
	}
	tmpArchive := filepath.Join(os.TempDir(), "redstone_jre_"+filepath.Base(url))
	errs := download.Pool(ctx, []download.Task{{ID: "jre", URLs: []string{url}, Dest: tmpArchive}}, download.Options{Workers: 1}, nil)
	if len(errs) > 0 {
		return errs[0]
	}
	defer os.Remove(tmpArchive)

	if strings.HasSuffix(url, ".zip") {
		return unzip(tmpArchive, destDir)
	}
	return untargz(tmpArchive, destDir)
}

func installMojangFiles(ctx context.Context, manifestURL, _ string, destDir string) error {
	body, err := httpGet(ctx, http.DefaultClient, manifestURL)
	if err != nil {
		return err
	}
	var man struct {
		Files map[string]struct {
			Type       string `json:"type"` // "file" / "directory" / "link"
			Target     string `json:"target,omitempty"`
			Executable bool   `json:"executable,omitempty"`
			Downloads  *struct {
				Raw struct {
					SHA1 string `json:"sha1"`
					Size int64  `json:"size"`
					URL  string `json:"url"`
				} `json:"raw"`
			} `json:"downloads,omitempty"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &man); err != nil {
		return err
	}

	var tasks []download.Task
	for relPath, f := range man.Files {
		full := filepath.Join(destDir, filepath.FromSlash(relPath))
		switch f.Type {
		case "directory":
			os.MkdirAll(full, 0o755)
		case "file":
			if f.Downloads == nil {
				continue
			}
			tasks = append(tasks, download.Task{
				ID: relPath, URLs: []string{f.Downloads.Raw.URL}, Dest: full,
				SHA1: f.Downloads.Raw.SHA1, Size: f.Downloads.Raw.Size, Executable: f.Executable,
			})
		case "link":
			os.MkdirAll(filepath.Dir(full), 0o755)
			os.Symlink(f.Target, full)
		}
	}
	errs := download.Pool(ctx, tasks, download.Options{Workers: 8}, nil)
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func tryAdoptium(ctx context.Context, client *http.Client, major int, plat Platform) string {
	osName := plat.OS
	if osName == "mac" {
		osName = "mac"
	}
	arch := plat.Arch
	if arch == "x64" {
		arch = "x64"
	}
	url := fmt.Sprintf(
		"https://api.adoptium.net/v3/binary/latest/%d/ga/%s/%s/jre/hotspot/normal/eclipse?project=jdk",
		major, osName, arch,
	)
	if !download.Ping(ctx, "https://api.adoptium.net", 5_000_000_000) {
		return ""
	}
	return url
}

func tryCorretto(major int, plat Platform) string {
	osName := map[string]string{"windows": "windows", "linux": "linux", "mac": "macos"}[plat.OS]
	arch := map[string]string{"x64": "x64", "arm64": "aarch64", "x86": "x86"}[plat.Arch]
	ext := "tar.gz"
	if plat.OS == "windows" {
		ext = "zip"
	}
	if osName == "" || arch == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://corretto.aws/downloads/latest/amazon-corretto-%d-%s-%s-jdk.%s",
		major, arch, osName, ext,
	)
}

func tryZulu(ctx context.Context, client *http.Client, major int, plat Platform) string {
	osName := map[string]string{"windows": "windows", "linux": "linux", "mac": "macosx"}[plat.OS]
	arch := map[string]string{"x64": "x64", "arm64": "aarch64", "x86": "i686"}[plat.Arch]
	ext := "tar.gz"
	if plat.OS == "windows" {
		ext = "zip"
	}
	if osName == "" || arch == "" {
		return ""
	}
	metaURL := fmt.Sprintf(
		"https://api.azul.com/metadata/v1/zulu/packages/?java_version=%d&os=%s&arch=%s&archive_type=%s&java_package_type=jre&javafx_bundled=false&latest=true&release_status=ga",
		major, osName, arch, ext,
	)
	body, err := httpGet(ctx, client, metaURL)
	if err != nil {
		return ""
	}
	var results []struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		return ""
	}
	return results[0].DownloadURL
}

func httpGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RedstoneCore/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func unzip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	root := commonZipRoot(r.File)
	for _, f := range r.File {
		rel := strings.TrimPrefix(f.Name, root)
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()|0o111)
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

func commonZipRoot(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	first := strings.SplitN(files[0].Name, "/", 2)[0]
	for _, f := range files {
		if !strings.HasPrefix(f.Name, first+"/") {
			return ""
		}
	}
	return first + "/"
}

func untargz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var root string
	first := true

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := hdr.Name
		if first {
			parts := strings.SplitN(name, "/", 2)
			root = parts[0] + "/"
			first = false
		}
		rel := strings.TrimPrefix(name, root)
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			io.Copy(out, tr)
			out.Close()
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}
