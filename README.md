<div align="center">

  <img src="logo.png" alt="Redstone Logo" width="120">

  # Redstone

  **High-performance, modular, and lightweight core library built specifically for powering custom Minecraft launchers.**

  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
  [![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)]()
  [![Go Reference](https://pkg.go.dev/badge/github.com/globthiss/redstone/core.svg)](https://pkg.go.dev/github.com/globthiss/redstone/core)
  [![Documentation](https://img.shields.io/badge/docs-/docs-green.svg)](/docs)

</div>

---

## 📦 Installation & Quick Start

### Installation

```bash
go get github.com/globthiss/redstone@latest
```

### Quick Start

```go
package main

import (
    "context"
    "github.com/globthiss/redstone/authutil"
    "github.com/globthiss/redstone/core"
    "github.com/globthiss/redstone/download"
    "github.com/globthiss/redstone/modes"
)

func main() {
    // Create event and progress channels
    events := make(chan core.StageEvent, 32)
    progress := make(chan download.Progress, 256)
    
    // Configure modes
    cfg := modes.Default()
    cfg.Java = modes.JavaModeSmart
    cfg.Memory = modes.MemoryModeAuto
    
    // Create session (offline mode)
    session := authutil.Session{
        Type:     authutil.AuthOffline,
        Username: "Player",
    }
    
    // Launch Minecraft
    launched, err := core.Launch(context.Background(), core.LaunchOptions{
        GameDir:   ".minecraft",
        VersionID: "1.20.1",
        Session:   session,
        Modes:     cfg,
        Events:    events,
        Progress:  progress,
    })
    if err != nil {
        panic(err)
    }
    
    // Wait for process completion
    launched.Process.Wait()
}
```

**Basic workflow:**
1. Initialize channels for events and download progress
2. Configure settings via `modes.Default()`
3. Create a session (offline/Microsoft/guest)
4. Call `core.Launch()` with required parameters
5. Work with the process via `launched.Process`

📚 **Full documentation** is available in the [`/docs`](/docs) folder.

---

## ⚡ Features

- **Secure account storage** — refresh tokens encrypted with AES-GCM, key auto-generated per installation
- **Multiple auth modes** — Offline, Microsoft (OAuth2 + Xbox Live chain), Anonymous guest, Compatibility
- **Smart Java runtime** — auto-detection of system/env/custom/embedded Java; fallback download from Mojang, Adoptium, Corretto, Zulu
- **Parallel downloads** — worker pool with retries, SHA1 verification, mirror fallback (bmclapi2 built-in), proxy support
- **Modular asset loading** — download only what you need: all / textures / sounds / lang / none
- **Library resolver** — Maven path resolution, native classifier per OS/arch, rule-based filtering
- **Full launch pipeline** — manifest → Java → libraries → assets → client.jar → natives → JVM args → process
- **Window control** — fullscreen, windowed, borderless, maximized with custom resolution
- **Cache management** — smart orphan cleanup, metadata wipe, or keep everything
- **Progress & events** — typed channels for download progress and stage events
- **Mirror system** — official, third-party, or hybrid chain with deduplication
- **Version icons** — auto-extract from client.jar, build .ico, create Windows shortcut
- **Flexible logging** — silent / console / file / debug / error modes
- **Launch modes** — Normal, Fast (skip hashes), Safe (verify Java), Test (dry-run), Server

---

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
