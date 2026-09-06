package mirrors

import (
	"strings"

	"redstone/modes"
)

type Kind int

const (
	KindVersionManifest Kind = iota
	KindVersionJSON
	KindClientJAR
	KindLibrary
	KindAsset
	KindJavaRuntime
)

type Config struct {
	Extra                 map[Kind][]string
	DisableBuiltinMirrors bool
	Mode                  modes.MirrorMode
}

var builtin = map[Kind][]struct {
	from string
	to   string
}{
	KindVersionManifest: {
		{"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com"},
		{"https://launchermeta.mojang.com", "https://bmclapi2.bangbang93.com"},
	},
	KindVersionJSON: {
		{"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com"},
		{"https://launchermeta.mojang.com", "https://bmclapi2.bangbang93.com"},
	},
	KindClientJAR: {
		{"https://piston-data.mojang.com", "https://bmclapi2.bangbang93.com"},
		{"https://launcher.mojang.com", "https://bmclapi2.bangbang93.com"},
	},
	KindLibrary: {
		{"https://libraries.minecraft.net", "https://bmclapi2.bangbang93.com/maven"},
	},
	KindAsset: {
		{"https://resources.download.minecraft.net", "https://bmclapi2.bangbang93.com/assets"},
	},
	KindJavaRuntime: {
		{"https://piston-meta.mojang.com", "https://bmclapi2.bangbang93.com"},
		{"https://launchermeta.mojang.com", "https://bmclapi2.bangbang93.com"},
	},
}

func (c Config) BuildChain(kind Kind, original string) []string {
	if c.Mode == modes.MirrorModeNone {
		return []string{original}
	}

	var mirrorURLs []string
	if !c.DisableBuiltinMirrors && c.Mode != modes.MirrorModeOfficial {
		for _, rule := range builtin[kind] {
			if strings.HasPrefix(original, rule.from) {
				mirrorURLs = append(mirrorURLs, strings.Replace(original, rule.from, rule.to, 1))
			}
		}
	}

	for _, extraBase := range c.Extra[kind] {
		for _, rule := range builtin[kind] {
			if strings.HasPrefix(original, rule.from) {
				mirrorURLs = append(mirrorURLs, strings.Replace(original, rule.from, extraBase, 1))
				break
			}
		}
	}

	var chain []string
	switch c.Mode {
	case modes.MirrorModeThirdParty:
		chain = mirrorURLs
	default:
		chain = append([]string{original}, mirrorURLs...)
	}

	return dedupe(chain)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

var PingEndpoints = []string{
	"https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
	"https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json",
}
