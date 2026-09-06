package accounts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Account struct {
	Username           string `json:"username"`
	UUID               string `json:"uuid"`
	EncryptedRefresh   string `json:"encrypted_refresh_token"`
	LastUsedUnixMillis int64  `json:"last_used"`
}

type Store struct {
	Path    string
	KeyPath string
}

func NewStore(gameDir string) *Store {
	return &Store{
		Path:    filepath.Join(gameDir, "accounts.json"),
		KeyPath: filepath.Join(gameDir, ".rc_keyfile"),
	}
}

type storeFile struct {
	Accounts map[string]Account `json:"accounts"`
}

func (s *Store) loadKey() ([]byte, error) {
	data, err := os.ReadFile(s.KeyPath)
	if err == nil && len(data) == 32 {
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.KeyPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.KeyPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) encrypt(plain string) (string, error) {
	key, err := s.loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func (s *Store) decrypt(encoded string) (string, error) {
	key, err := s.loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("повреждённые данные аккаунта")
	}
	nonce, cipherText := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Store) load() (*storeFile, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeFile{Accounts: map[string]Account{}}, nil
		}
		return nil, err
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	if sf.Accounts == nil {
		sf.Accounts = map[string]Account{}
	}
	return &sf, nil
}

func (s *Store) save(sf *storeFile) error {
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}

func (s *Store) SaveRefreshToken(username, uuid, refreshToken string) error {
	sf, err := s.load()
	if err != nil {
		return err
	}
	enc, err := s.encrypt(refreshToken)
	if err != nil {
		return err
	}
	sf.Accounts[username] = Account{
		Username:         username,
		UUID:             uuid,
		EncryptedRefresh: enc,
	}
	return s.save(sf)
}

func (s *Store) GetRefreshToken(username string) (string, error) {
	sf, err := s.load()
	if err != nil {
		return "", err
	}
	acc, ok := sf.Accounts[username]
	if !ok {
		return "", fmt.Errorf("аккаунт %s не найден в accounts.json", username)
	}
	return s.decrypt(acc.EncryptedRefresh)
}

func (s *Store) List() ([]Account, error) {
	sf, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(sf.Accounts))
	for _, a := range sf.Accounts {
		out = append(out, a)
	}
	return out, nil
}

func (s *Store) Remove(username string) error {
	sf, err := s.load()
	if err != nil {
		return err
	}
	delete(sf.Accounts, username)
	return s.save(sf)
}
