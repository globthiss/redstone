package modes

type JavaMode string

const (
	JavaModeSystem   JavaMode = "system"
	JavaModeAuto     JavaMode = "auto"
	JavaModeCustom   JavaMode = "custom"
	JavaModeSmart    JavaMode = "smart"
	JavaModeManual   JavaMode = "manual"
	JavaModeEmbedded JavaMode = "embedded"
	JavaModeEnv      JavaMode = "env"
)

type JavaSource string

const (
	JavaSourceSystem   JavaSource = "system"
	JavaSourceAuto     JavaSource = "auto"
	JavaSourceCustom   JavaSource = "custom"
	JavaSourceManual   JavaSource = "manual"
	JavaSourceEnv      JavaSource = "env"
	JavaSourceEmbedded JavaSource = "embedded"
	JavaSourceCache    JavaSource = "cache"
	JavaSourcePortable JavaSource = "portable"
)

type DownloadMode string

const (
	DownloadModeSequential     DownloadMode = "sequential"
	DownloadModeParallel       DownloadMode = "parallel"
	DownloadModeSmart          DownloadMode = "smart"
	DownloadModeCacheOnly      DownloadMode = "cache_only"
	DownloadModeMirrorPriority DownloadMode = "mirror_priority"
)

type NetworkMode string

const (
	NetworkModeAuto    NetworkMode = "auto"
	NetworkModeOnline  NetworkMode = "online"
	NetworkModeOffline NetworkMode = "offline"
	NetworkModeHybrid  NetworkMode = "hybrid"
	NetworkModeProxy   NetworkMode = "proxy"
)

type AuthMode string

const (
	AuthModeOffline       AuthMode = "offline"
	AuthModeMicrosoft     AuthMode = "microsoft"
	AuthModeMojang        AuthMode = "mojang"
	AuthModeCompatibility AuthMode = "compatibility"
	AuthModeAnonymous     AuthMode = "anonymous"
	AuthModeAuto          AuthMode = "auto"
)

type VersionType string

const (
	VersionTypeVanilla  VersionType = "vanilla"
	VersionTypeLegacy   VersionType = "legacy"
	VersionTypeSnapshot VersionType = "snapshot"
	VersionTypeCustom   VersionType = "custom"
)

type LogMode string

const (
	LogModeConsole LogMode = "console"
	LogModeFile    LogMode = "file"
	LogModeBoth    LogMode = "both"
	LogModeDebug   LogMode = "debug"
	LogModeError   LogMode = "error"
	LogModeSilent  LogMode = "silent"
)

type CacheMode string

const (
	CacheModeAll      CacheMode = "all"
	CacheModeCore     CacheMode = "core"
	CacheModeMetadata CacheMode = "metadata"
	CacheModeNone     CacheMode = "none"
	CacheModeSmart    CacheMode = "smart"
)

type MemoryMode string

const (
	MemoryModeManual   MemoryMode = "manual"
	MemoryModeAuto     MemoryMode = "auto"
	MemoryModeMinimal  MemoryMode = "minimal"
	MemoryModeMaximum  MemoryMode = "maximum"
	MemoryModeBalanced MemoryMode = "balanced"
)

type MirrorMode string

const (
	MirrorModeOfficial   MirrorMode = "official"
	MirrorModeThirdParty MirrorMode = "thirdparty"
	MirrorModeAll        MirrorMode = "all"
	MirrorModeSmart      MirrorMode = "smart"
	MirrorModeNone       MirrorMode = "none"
)

type LibraryMode string

const (
	LibraryModeAll       LibraryMode = "all"
	LibraryModeMinimal   LibraryMode = "minimal"
	LibraryModeNative    LibraryMode = "native"
	LibraryModeCurrentOS LibraryMode = "current_os"
)

type AssetMode string

const (
	AssetModeAll      AssetMode = "all"
	AssetModeMinimal  AssetMode = "minimal"
	AssetModeSounds   AssetMode = "sounds"
	AssetModeTextures AssetMode = "textures"
	AssetModeNone     AssetMode = "none"
)

type AccountType string

const (
	AccountTypeOffline   AccountType = "offline"
	AccountTypeMicrosoft AccountType = "microsoft"
	AccountTypeGuest     AccountType = "guest"
	AccountTypeDemo      AccountType = "demo"
	AccountTypeService   AccountType = "service"
)

type LaunchMode string

const (
	LaunchModeNormal LaunchMode = "normal"
	LaunchModeFast   LaunchMode = "fast"
	LaunchModeSafe   LaunchMode = "safe"
	LaunchModeTest   LaunchMode = "test"
	LaunchModeServer LaunchMode = "server"
)

type WindowMode string

const (
	WindowModeFullscreen WindowMode = "fullscreen"
	WindowModeWindowed   WindowMode = "windowed"
	WindowModeBorderless WindowMode = "borderless"
	WindowModeMinimal    WindowMode = "minimal"
	WindowModeMaximized  WindowMode = "maximized"
)

type Config struct {
	Java            JavaMode
	JavaCustomPath  string
	JavaEmbeddedDir string
	Download        DownloadMode
	Network         NetworkMode
	ProxyURL        string
	Auth            AuthMode
	Version         VersionType
	Log             LogMode
	LogFilePath     string
	Cache           CacheMode
	Memory          MemoryMode
	Mirror          MirrorMode
	Library         LibraryMode
	Asset           AssetMode
	Account         AccountType
	Launch          LaunchMode
	Window          WindowMode
}

func Default() Config {
	return Config{
		Java:     JavaModeSmart,
		Download: DownloadModeParallel,
		Network:  NetworkModeAuto,
		Auth:     AuthModeOffline,
		Version:  VersionTypeVanilla,
		Log:      LogModeBoth,
		Cache:    CacheModeAll,
		Memory:   MemoryModeAuto,
		Mirror:   MirrorModeAll,
		Library:  LibraryModeCurrentOS,
		Asset:    AssetModeAll,
		Account:  AccountTypeOffline,
		Launch:   LaunchModeNormal,
		Window:   WindowModeWindowed,
	}
}
