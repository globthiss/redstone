package authutil

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"redstone/accounts"
	"redstone/modes"
)

type AuthType string

const (
	AuthOffline   AuthType = "offline"
	AuthMicrosoft AuthType = "microsoft"
)

type Session struct {
	Type        AuthType
	Username    string
	UUID        string
	AccessToken string
	XUID        string
	Account     modes.AccountType
	Demo        bool
}

func OfflineUUID(username string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + username))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func randomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Session) Normalize() {
	if s.Type == AuthOffline {
		if s.UUID == "" {
			s.UUID = OfflineUUID(s.Username)
		}
		if s.AccessToken == "" {
			s.AccessToken = "0"
		}
		if s.Account == "" {
			s.Account = modes.AccountTypeOffline
		}
	}
}

func BuildSession(mode modes.AuthMode, username, gameDir string, compat *Session) (*Session, error) {
	switch mode {
	case modes.AuthModeOffline:
		s := &Session{Type: AuthOffline, Username: username, Account: modes.AccountTypeOffline}
		s.Normalize()
		return s, nil

	case modes.AuthModeAnonymous:
		s := &Session{
			Type:        AuthOffline,
			Username:    fmt.Sprintf("Guest%d", randomSuffix()),
			AccessToken: "0",
			Account:     modes.AccountTypeGuest,
		}
		s.Normalize()
		return s, nil

	case modes.AuthModeCompatibility:
		if compat == nil {
			return nil, fmt.Errorf("для режима compatibility нужно передать готовую Session (compat) с externally-issued accessToken/uuid")
		}
		compat.Account = modes.AccountTypeService
		return compat, nil

	case modes.AuthModeMojang:
		return nil, fmt.Errorf("legacy Mojang-авторизация отключена Mojang с 2021 года и не поддерживается")

	case modes.AuthModeMicrosoft:
		return nil, fmt.Errorf("для microsoft авторизации используйте пакет msauth напрямую (нужен интерактивный флоу или сохранённый refresh token)")

	case modes.AuthModeAuto:
		store := accounts.NewStore(gameDir)
		list, err := store.List()
		if err != nil || len(list) == 0 {
			return nil, fmt.Errorf("нет сохранённых аккаунтов для авто-входа, требуется явный вход")
		}
		last := list[0]
		return nil, fmt.Errorf("для авто-входа с сохранённым аккаунтом %s используйте msauth.LoginWithRefreshToken с токеном из accounts-хранилища", last.Username)

	default:
		return nil, fmt.Errorf("неизвестный режим авторизации: %s", mode)
	}
}

func randomSuffix() int {
	b := make([]byte, 2)
	rand.Read(b)
	return int(b[0])<<8 | int(b[1])
}
