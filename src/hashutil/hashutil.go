package hashutil

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
)

func FileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func VerifyFile(path string, expectedSHA1 string, expectedSize int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return false
	}
	if expectedSHA1 == "" {
		return true
	}
	sum, err := FileSHA1(path)
	if err != nil {
		return false
	}
	return sum == expectedSHA1
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
