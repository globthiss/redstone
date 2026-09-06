package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"redstone/assets"
	"redstone/authutil"
	"redstone/cache"
	"redstone/download"
	"redstone/gameargs"
	"redstone/hashutil"
	"redstone/iconfix"
	"redstone/javart"
	"redstone/libraries"
	"redstone/logging"
	"redstone/manifest"
	"redstone/mirrors"
	"redstone/modes"
	"redstone/natives"
	"redstone/process"
)

type StageEvent struct {
	Stage   string
	Message string
	Err     error
}

type LaunchOptions struct {
	GameDir         string
	VersionID       string
	Session         authutil.Session
	Mirrors         mirrors.Config
	Platform        javart.Platform
	Modes           modes.Config
	XmxMB           int
	XmsMB           int
	Width           int
	Height          int
	ScreenWidth     int
	ScreenHeight    int
	ExtraJVM        []string
	LauncherName    string
	LauncherVersion string
	ForceOffline    bool

	Events   chan<- StageEvent
	Progress chan<- download.Progress
}

type Launched struct {
	Process *process.Handle
	Cleanup func()
	Version *manifest.Version
	DryRun  bool
}

func emit(ch chan<- StageEvent, log *logging.Logger, stage, msg string, err error) {
	if err != nil {
		if log != nil {
			log.Error("[%s] %s: %v", stage, msg, err)
		}
	} else if log != nil {
		log.Info("[%s] %s", stage, msg)
	}
	if ch == nil {
		return
	}
	select {
	case ch <- StageEvent{Stage: stage, Message: msg, Err: err}:
	default:
	}
}

func Launch(ctx context.Context, o LaunchOptions) (*Launched, error) {
	if o.Platform.OS == "" {
		o.Platform = javart.CurrentPlatform()
	}
	if o.Modes == (modes.Config{}) {
		o.Modes = modes.Default()
	}
	if o.Mirrors.Mode == "" {
		o.Mirrors.Mode = o.Modes.Mirror
	}

	var logFilePath string
	if o.Modes.LogFilePath != "" {
		logFilePath = o.Modes.LogFilePath
	} else {
		logFilePath = filepath.Join(o.GameDir, "redstonecore.log")
	}
	logger, err := logging.New(o.Modes.Log, logFilePath)
	if err != nil {
		logger = nil
	}
	if logger != nil {
		defer logger.Close()
	}

	emit(o.Events, logger, "init", "Проверка структуры .minecraft", nil)
	for _, sub := range []string{"versions", "libraries", "assets", "runtime"} {
		if err := os.MkdirAll(filepath.Join(o.GameDir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("не удалось создать структуру папок: %w", err)
		}
	}

	if o.Modes.Cache == modes.CacheModeNone || o.Modes.Cache == modes.CacheModeMetadata {
		cacheMgr := cache.New(o.GameDir)
		if err := cacheMgr.Apply(o.Modes.Cache); err != nil {
			emit(o.Events, logger, "init", "Не удалось применить режим кэша", err)
		}
	}

	online := resolveNetwork(ctx, o)
	emit(o.Events, logger, "init", fmt.Sprintf("Режим: %s", modeLabel(online)), nil)

	emit(o.Events, logger, "manifest", "Получение данных о версии "+o.VersionID, nil)
	mm := &manifest.Manager{GameDir: o.GameDir, Mirrors: o.Mirrors, Online: online}

	var entryURL, entrySHA1 string
	if online {
		vm, err := mm.FetchVersionManifest(ctx)
		if err == nil {
			for _, v := range vm.Versions {
				if v.ID == o.VersionID {
					entryURL, entrySHA1 = v.URL, v.SHA1
					break
				}
			}
		}
	}
	version, err := mm.FetchVersion(ctx, o.VersionID, entryURL, entrySHA1)
	if err != nil {
		return nil, fmt.Errorf("этап 2 (манифест): %w", err)
	}
	if version.Downloads == nil {
		version.Downloads = map[string]manifest.DownloadArtifact{}
	}

	requiredJava := version.RequiredJavaMajor()
	emit(o.Events, logger, "java", fmt.Sprintf("Проверка Java %d", requiredJava), nil)
	jm := &javart.Manager{
		RuntimesDir: filepath.Join(o.GameDir, "runtime"),
		Online:      online,
		Mode:        o.Modes.Java,
		CustomPath:  o.Modes.JavaCustomPath,
		EmbeddedDir: o.Modes.JavaEmbeddedDir,
	}
	javaResolved, err := jm.Resolve(ctx, requiredJava, o.Platform)
	if err != nil {
		return nil, fmt.Errorf("этап 3 (java runtime): %w", err)
	}

	if o.Modes.Launch == modes.LaunchModeSafe {
		if err := exec.Command(javaResolved.JavaBinary, "-version").Run(); err != nil {
			return nil, fmt.Errorf("этап 3 (safe-проверка java): бинарь не запускается: %w", err)
		}
	}

	emit(o.Events, logger, "libraries", "Проверка библиотек", nil)
	filtered := libraries.FilterByMode(version.Libraries, o.Platform, o.Modes.Library)
	librariesDir := filepath.Join(o.GameDir, "libraries")
	resolvedLibs, libTasks := libraries.Resolve(filtered, librariesDir, o.Platform, o.Mirrors)
	if o.Modes.Launch == modes.LaunchModeFast {
		libTasks = stripHashes(libTasks)
	}
	dlOpts := download.Options{Workers: 8, Retries: 3, Mode: o.Modes.Download, ProxyURL: o.Modes.ProxyURL}
	if online {
		if errs := download.Pool(ctx, libTasks, dlOpts, o.Progress); len(errs) > 0 {
			return nil, fmt.Errorf("этап 4 (библиотеки): %d ошибок, первая: %w", len(errs), errs[0])
		}
	} else {
		download.Pool(ctx, libTasks, download.Options{Workers: 8, Mode: modes.DownloadModeCacheOnly}, o.Progress)
	}

	assetsDir := filepath.Join(o.GameDir, "assets")
	if o.Modes.Asset == modes.AssetModeNone || assets.IsPreAssetsEra(version.Assets) {
		emit(o.Events, logger, "assets", "Ассеты пропущены (режим none или версия pre-1.6)", nil)
	} else {
		emit(o.Events, logger, "assets", "Проверка ассетов ("+version.Assets+")", nil)
		am := &assets.Manager{GameDir: o.GameDir, Mirrors: o.Mirrors, Online: online, Mode: o.Modes.Asset}
		idxPath, err := am.EnsureIndex(ctx, version.AssetIndex)
		if err != nil {
			return nil, fmt.Errorf("этап 5 (индекс ассетов): %w", err)
		}
		if err := am.EnsureObjects(ctx, idxPath, version.Assets, o.Progress); err != nil {
			emit(o.Events, logger, "assets", "Внимание: часть ассетов не загружена", err)
		}
	}

	versionDir := filepath.Join(o.GameDir, "versions", o.VersionID)
	clientJarPath := filepath.Join(versionDir, o.VersionID+".jar")
	emit(o.Events, logger, "client", "Проверка client.jar", nil)
	if clientArt, ok := version.Downloads["client"]; ok && clientArt.URL != "" {
		chain := o.Mirrors.BuildChain(mirrors.KindClientJAR, clientArt.URL)
		sha1 := clientArt.SHA1
		if o.Modes.Launch == modes.LaunchModeFast {
			sha1 = ""
		}
		task := download.Task{ID: "client.jar", URLs: chain, Dest: clientJarPath, SHA1: sha1, Size: clientArt.Size}
		if !download.VerifyCached(task) {
			if !online {
				return nil, fmt.Errorf("этап 6: client.jar отсутствует, а сети нет")
			}
			if errs := download.Pool(ctx, []download.Task{task}, download.Options{Workers: 1, Mode: o.Modes.Download}, o.Progress); len(errs) > 0 {
				return nil, fmt.Errorf("этап 6 (client.jar): %w", errs[0])
			}
		}
	} else if !hashutil.Exists(clientJarPath) {
		return nil, fmt.Errorf("этап 6: не удалось определить URL client.jar и локального файла тоже нет")
	}

	var icoPath string
	if o.Modes.Launch != modes.LaunchModeServer {
		icoPath, _ = iconfix.EnsureVersionIcon(clientJarPath, versionDir)
	}

	emit(o.Events, logger, "natives", "Подготовка нативных библиотек", nil)
	extractor := &natives.Extractor{}
	nativesDir, err := extractor.CreateTempDir()
	if err != nil {
		return nil, fmt.Errorf("этап 7 (создание temp папки): %w", err)
	}
	var nativeLibs []libraries.Resolved
	for _, l := range resolvedLibs {
		if l.IsNative {
			nativeLibs = append(nativeLibs, l)
		}
	}
	if err := extractor.ExtractAll(nativeLibs, filtered, o.Platform.OS); err != nil {
		extractor.Cleanup()
		return nil, fmt.Errorf("этап 7 (распаковка нативов): %w", err)
	}

	emit(o.Events, logger, "launch", "Сборка команды запуска "+o.VersionID, nil)
	classPath := libraries.ClassPath(resolvedLibs, clientJarPath, o.Platform)

	xmx, xms := gameargs.ResolveMemoryMB(o.Modes.Memory, o.XmxMB, o.XmsMB)

	windowMode := o.Modes.Window
	if o.Modes.Launch == modes.LaunchModeServer {
		windowMode = modes.WindowModeMinimal
	}

	buildOpts := gameargs.BuildOptions{
		Version: version, Session: o.Session, GameDir: o.GameDir, AssetsDir: assetsDir,
		AssetsIndexID: version.Assets, ClassPath: classPath, NativesDir: nativesDir,
		Platform: o.Platform, LauncherName: o.LauncherName, LauncherVersion: o.LauncherVersion,
		WindowWidth: o.Width, WindowHeight: o.Height, WindowMode: windowMode,
		ScreenWidth: o.ScreenWidth, ScreenHeight: o.ScreenHeight,
		ExtraJVMArgs: o.ExtraJVM, XmxMB: xmx, XmsMB: xms,
		Demo: o.Session.Account == modes.AccountTypeDemo,
	}

	var fullArgs []string
	fullArgs = append(fullArgs, gameargs.BuildJVMArgs(buildOpts)...)
	fullArgs = append(fullArgs, version.MainClass)
	fullArgs = append(fullArgs, gameargs.BuildGameArgs(buildOpts)...)
	if o.Modes.Launch == modes.LaunchModeServer {
		fullArgs = append(fullArgs, "--nogui")
	}

	if o.Platform.OS == "windows" && icoPath != "" && o.Modes.Launch != modes.LaunchModeServer {
		lnkPath := filepath.Join(versionDir, o.VersionID+".lnk")
		iconfix.CreateLaunchShortcut(lnkPath, javaResolved.JavaBinary, joinArgs(fullArgs), o.GameDir, icoPath)
	}

	if o.Modes.Cache == modes.CacheModeSmart {
		go cache.New(o.GameDir).Apply(modes.CacheModeSmart)
	}

	if o.Modes.Launch == modes.LaunchModeTest {
		emit(o.Events, logger, "launch", "Тестовый режим: все проверки пройдены, процесс не запускается", nil)
		return &Launched{Version: version, DryRun: true, Cleanup: func() { extractor.Cleanup() }}, nil
	}

	emit(o.Events, logger, "launch", "Запуск Minecraft "+o.VersionID, nil)
	handle, err := process.Start(ctx, process.Options{
		JavaBinary: javaResolved.JavaBinary,
		Args:       fullArgs,
		WorkDir:    o.GameDir,
		Cleanup:    func() { extractor.Cleanup() },
	})
	if err != nil {
		extractor.Cleanup()
		return nil, fmt.Errorf("этап 8 (запуск процесса): %w", err)
	}

	return &Launched{Process: handle, Cleanup: func() { extractor.Cleanup() }, Version: version}, nil
}

func resolveNetwork(ctx context.Context, o LaunchOptions) bool {
	if o.ForceOffline || o.Modes.Network == modes.NetworkModeOffline {
		return false
	}
	if o.Modes.Network == modes.NetworkModeOnline {
		return true
	}

	pingCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	for _, url := range mirrors.PingEndpoints {
		if download.Ping(pingCtx, url, 3*time.Second) {
			return true
		}
	}
	return false
}

func stripHashes(tasks []download.Task) []download.Task {
	out := make([]download.Task, len(tasks))
	for i, t := range tasks {
		t.SHA1 = ""
		out[i] = t
	}
	return out
}

func modeLabel(online bool) string {
	if online {
		return "онлайн (синхронизация с Mojang/зеркалами)"
	}
	return "офлайн (используются только локальные файлы)"
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		if containsSpace(a) {
			out += "\"" + a + "\""
		} else {
			out += a
		}
	}
	return out
}

func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}
