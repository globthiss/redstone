package gameargs

import (
	"strconv"
	"strings"

	"redstone/authutil"
	"redstone/javart"
	"redstone/libraries"
	"redstone/manifest"
	"redstone/modes"
)

type BuildOptions struct {
	Version         *manifest.Version
	Session         authutil.Session
	GameDir         string
	AssetsDir       string
	AssetsIndexID   string
	ClassPath       string
	NativesDir      string
	Platform        javart.Platform
	LauncherName    string
	LauncherVersion string
	WindowWidth     int
	WindowHeight    int
	WindowMode      modes.WindowMode
	ScreenWidth     int
	ScreenHeight    int
	ExtraJVMArgs    []string
	XmxMB           int
	XmsMB           int
	Demo            bool
}

func placeholders(o BuildOptions) map[string]string {
	o.Session.Normalize()
	m := map[string]string{
		"auth_player_name":    o.Session.Username,
		"auth_uuid":           o.Session.UUID,
		"auth_access_token":   o.Session.AccessToken,
		"auth_session":        o.Session.AccessToken,
		"user_type":           map[bool]string{true: "msa", false: "legacy"}[o.Session.Type == authutil.AuthMicrosoft],
		"user_properties":     "{}",
		"version_name":        o.Version.ID,
		"version_type":        o.Version.Type,
		"game_directory":      o.GameDir,
		"assets_root":         o.AssetsDir,
		"game_assets":         o.AssetsDir, // для очень старых версий (pre-1.7.10 иногда используют этот ключ)
		"assets_index_name":   o.AssetsIndexID,
		"natives_directory":   o.NativesDir,
		"launcher_name":       defaultStr(o.LauncherName, "RedstoneCore"),
		"launcher_version":    defaultStr(o.LauncherVersion, "1.0.0"),
		"classpath":           o.ClassPath,
		"classpath_separator": map[bool]string{true: ";", false: ":"}[o.Platform.OS == "windows"],
		"library_directory":   o.GameDir + "/libraries",
		"clientid":            "",
		"resolution_width":    itoaOr(o.WindowWidth, "854"),
		"resolution_height":   itoaOr(o.WindowHeight, "480"),
	}
	return m
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func itoaOr(v int, def string) string {
	if v <= 0 {
		return def
	}
	return strconv.Itoa(v)
}

func substitute(template string, ph map[string]string) string {
	pairs := make([]string, 0, len(ph)*2)
	for k, v := range ph {
		pairs = append(pairs, "${"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

func BuildJVMArgs(o BuildOptions) []string {
	ph := placeholders(o)
	var out []string

	if o.XmsMB > 0 {
		out = append(out, "-Xms"+strconv.Itoa(o.XmsMB)+"M")
	}
	if o.XmxMB > 0 {
		out = append(out, "-Xmx"+strconv.Itoa(o.XmxMB)+"M")
	}

	out = append(out, "-Djava.library.path="+o.NativesDir)
	out = append(out, "-Djna.tmpdir="+o.NativesDir)
	out = append(out, "-Dorg.lwjgl.system.SharedLibraryExtractPath="+o.NativesDir)
	out = append(out, "-Dio.netty.native.workdir="+o.NativesDir)

	if o.Platform.OS == "mac" {
		out = append(out, "-Xdock:name=Minecraft", "-XstartOnFirstThread")
	}

	if o.Version.Arguments != nil {
		for _, a := range o.Version.Arguments.JVM {
			if !libraries.EvaluateRules(a.Rules, o.Platform) {
				continue
			}
			for _, v := range a.Value {
				out = append(out, substitute(v, ph))
			}
		}
	} else {
	}

	out = append(out, o.ExtraJVMArgs...)
	out = append(out, "-cp", o.ClassPath)

	if o.Version.Logging != nil && o.Version.Logging.Client.Argument != "" {
		logCfgPath := o.GameDir + "/assets/log_configs/" + o.Version.Logging.Client.File.SHA1 + "-" + o.Version.ID + ".xml"
		out = append(out, substitute(o.Version.Logging.Client.Argument, map[string]string{"path": logCfgPath}))
	}

	return out
}

func BuildGameArgs(o BuildOptions) []string {
	ph := placeholders(o)
	var out []string

	if o.Version.IsLegacyArguments() {
		for _, tok := range strings.Fields(o.Version.MinecraftArguments) {
			out = append(out, substitute(tok, ph))
		}
		if len(out) == 0 {
			out = []string{
				o.Session.Username,
				o.Session.AccessToken,
				"--gameDir", o.GameDir,
				"--assetsDir", o.AssetsDir,
			}
		}
	} else if o.Version.Arguments != nil {
		for _, a := range o.Version.Arguments.Game {
			if !libraries.EvaluateRules(a.Rules, o.Platform) {
				continue
			}
			for _, v := range a.Value {
				out = append(out, substitute(v, ph))
			}
		}
	}

	if o.WindowWidth > 0 && o.WindowHeight > 0 && !containsFlag(out, "--width") {
		out = append(out, "--width", strconv.Itoa(o.WindowWidth), "--height", strconv.Itoa(o.WindowHeight))
	}

	switch o.WindowMode {
	case modes.WindowModeFullscreen:
		out = append(out, "--fullscreen")
	case modes.WindowModeBorderless, modes.WindowModeMaximized:
		if o.ScreenWidth > 0 && o.ScreenHeight > 0 {
			out = replaceResolution(out, o.ScreenWidth, o.ScreenHeight)
		}
	}

	if o.Demo {
		out = append(out, "--demo")
	}

	return out
}

func replaceResolution(args []string, width, height int) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--width" && i+1 < len(args) {
			out = append(out, "--width", strconv.Itoa(width))
			i++
			continue
		}
		if args[i] == "--height" && i+1 < len(args) {
			out = append(out, "--height", strconv.Itoa(height))
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
