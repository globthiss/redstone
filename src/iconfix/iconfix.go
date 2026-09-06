package iconfix

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

var candidateIconPaths = [][]string{
	{"icons/icon_32x32.png", "icons/icon_16x16.png"}, // современный формат
	{"pack.png"}, // очень старые версии иногда кладут иконку сюда
}

func ExtractFromClientJar(clientJarPath, iconsDir string) ([]string, error) {
	r, err := zip.OpenReader(clientJarPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть client.jar: %w", err)
	}
	defer r.Close()

	index := map[string]*zip.File{}
	for _, f := range r.File {
		index[f.Name] = f
	}

	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		return nil, err
	}

	var saved []string
	for _, group := range candidateIconPaths {
		for _, name := range group {
			f, ok := index[name]
			if !ok {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			outPath := filepath.Join(iconsDir, filepath.Base(name))
			if err := os.WriteFile(outPath, data, 0o644); err == nil {
				saved = append(saved, outPath)
			}
		}
		if len(saved) > 0 {
			break // нашли рабочую группу - дальше не ищем
		}
	}

	return saved, nil
}

func BuildICO(pngPaths []string, destICO string) error {
	if len(pngPaths) == 0 {
		return fmt.Errorf("нет исходных PNG для сборки .ico")
	}

	type entry struct {
		width, height int
		data          []byte
	}
	var entries []entry

	for _, p := range pngPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			continue
		}
		entries = append(entries, entry{width: cfg.Width, height: cfg.Height, data: data})
	}
	if len(entries) == 0 {
		return fmt.Errorf("не удалось декодировать ни одну иконку")
	}

	var buf bytes.Buffer
	writeU16 := func(v uint16) { buf.WriteByte(byte(v)); buf.WriteByte(byte(v >> 8)) }
	writeU32 := func(v uint32) {
		buf.WriteByte(byte(v))
		buf.WriteByte(byte(v >> 8))
		buf.WriteByte(byte(v >> 16))
		buf.WriteByte(byte(v >> 24))
	}

	writeU16(0)
	writeU16(1)
	writeU16(uint16(len(entries)))

	headerSize := 6 + 16*len(entries)
	offset := uint32(headerSize)

	for _, e := range entries {
		w := e.width
		h := e.height
		if w > 255 {
			w = 0 // 0 означает 256 в формате ICO
		}
		if h > 255 {
			h = 0
		}
		buf.WriteByte(byte(w))
		buf.WriteByte(byte(h))
		buf.WriteByte(0) // палитра
		buf.WriteByte(0) // reserved
		writeU16(1)      // color planes
		writeU16(32)     // bits per pixel
		writeU32(uint32(len(e.data)))
		writeU32(offset)
		offset += uint32(len(e.data))
	}

	for _, e := range entries {
		buf.Write(e.data)
	}

	if err := os.MkdirAll(filepath.Dir(destICO), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destICO, buf.Bytes(), 0o644)
}

func EnsureVersionIcon(clientJarPath, versionDir string) (string, error) {
	icoPath := filepath.Join(versionDir, "icon.ico")
	if _, err := os.Stat(icoPath); err == nil {
		return icoPath, nil
	}

	pngs, err := ExtractFromClientJar(clientJarPath, filepath.Join(versionDir, "icons"))
	if err != nil {
		return "", err
	}
	if len(pngs) == 0 {
		return "", nil
	}

	valid := pngs[:0]
	for _, p := range pngs {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		_, _, err = image.DecodeConfig(f)
		f.Close()
		if err == nil {
			valid = append(valid, p)
		}
	}
	if len(valid) == 0 {
		return "", nil
	}

	if err := BuildICO(valid, icoPath); err != nil {
		return "", err
	}
	return icoPath, nil
}
