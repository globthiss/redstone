package msauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	authorizeURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"
	tokenURL     = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	xblAuthURL   = "https://user.auth.xboxlive.com/user/authenticate"
	xstsAuthURL  = "https://xsts.auth.xboxlive.com/xsts/authorize"
	mcLoginURL   = "https://api.minecraftservices.com/authentication/login_with_xbox"
	mcProfileURL = "https://api.minecraftservices.com/minecraft/profile"
	mcEntitleURL = "https://api.minecraftservices.com/entitlements/mcstore"
	scope        = "XboxLive.signin offline_access"
)

type Config struct {
	ClientID    string
	RedirectURI string
	HTTPClient  *http.Client
}

func (c *Config) fill() {
    if c.ClientID == "" {
        c.ClientID = "00000000402b5328"
    }
    if c.RedirectURI == "" {
        c.RedirectURI = "https://login.live.com/oauth20_desktop.srf"
    }
    if c.HTTPClient == nil {
        c.HTTPClient = &http.Client{Timeout: 20 * time.Second}
    }
}

type MSTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type Result struct {
	AccessToken  string
	RefreshToken string
	UUID         string
	Username     string
	XUID         string
}

func (c *Config) LoginInteractive(ctx context.Context) (*Result, error) {
	c.fill()

	redirectURL, err := url.Parse(c.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("некорректный redirect uri: %w", err)
	}
	port := redirectURL.Port()
	if port == "" {
		port = "8934"
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var once sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc(redirectURL.Path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			once.Do(func() {
				errCh <- fmt.Errorf("microsoft вернул ошибку: %s - %s", errStr, q.Get("error_description"))
			})
			fmt.Fprint(w, "Авторизация не удалась, можно закрыть окно.")
			return
		}
		code := q.Get("code")
		if code == "" {
			once.Do(func() { errCh <- fmt.Errorf("код авторизации не получен") })
			fmt.Fprint(w, "Ошибка: код не найден в запросе.")
			return
		}
		once.Do(func() { codeCh <- code })
		fmt.Fprint(w, "Авторизация успешна, можно закрыть это окно и вернуться в лаунчер.")
	})

	server := &http.Server{Addr: ":" + port, Handler: mux}
	go server.ListenAndServe()
	defer server.Close()

	authURL := buildAuthURL(c.ClientID, c.RedirectURI)
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("не удалось открыть браузер: %w (откройте вручную: %s)", err, authURL)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("превышено время ожидания авторизации")
	}

	msTokens, err := c.exchangeCodeForToken(ctx, code)
	if err != nil {
		return nil, err
	}
	return c.completeChain(ctx, msTokens)
}

func (c *Config) LoginWithRefreshToken(ctx context.Context, refreshToken string) (*Result, error) {
	c.fill()

	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", scope)
	form.Set("redirect_uri", c.RedirectURI)

	var msTokens MSTokens
	if err := c.postForm(ctx, tokenURL, form, &msTokens); err != nil {
		return nil, fmt.Errorf("не удалось обновить токен по refresh_token: %w", err)
	}
	return c.completeChain(ctx, &msTokens)
}

func (c *Config) exchangeCodeForToken(ctx context.Context, code string) (*MSTokens, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURI)
	form.Set("scope", scope)

	var tokens MSTokens
	if err := c.postForm(ctx, tokenURL, form, &tokens); err != nil {
		return nil, fmt.Errorf("этап 3 (microsoft token): %w", err)
	}
	return &tokens, nil
}

type xblRequest struct {
	Properties struct {
		AuthMethod string `json:"AuthMethod"`
		SiteName   string `json:"SiteName"`
		RpsTicket  string `json:"RpsTicket"`
	} `json:"Properties"`
	RelyingParty string `json:"RelyingParty"`
	TokenType    string `json:"TokenType"`
}

type xblResponse struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			Uhs string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

type xstsRequest struct {
	Properties struct {
		SandboxID  string   `json:"SandboxId"`
		UserTokens []string `json:"UserTokens"`
	} `json:"Properties"`
	RelyingParty string `json:"RelyingParty"`
	TokenType    string `json:"TokenType"`
}

type mcLoginRequest struct {
	IdentityToken string `json:"identityToken"`
}

type mcLoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type mcProfileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Config) completeChain(ctx context.Context, msTokens *MSTokens) (*Result, error) {
	var xbl xblResponse
	xblReq := xblRequest{RelyingParty: "http://auth.xboxlive.com", TokenType: "JWT"}
	xblReq.Properties.AuthMethod = "RPS"
	xblReq.Properties.SiteName = "user.auth.xboxlive.com"
	xblReq.Properties.RpsTicket = "d=" + msTokens.AccessToken
	if err := c.postJSON(ctx, xblAuthURL, xblReq, &xbl); err != nil {
		return nil, fmt.Errorf("этап 4 (xbox live token): %w", err)
	}
	if xbl.Token == "" || len(xbl.DisplayClaims.Xui) == 0 {
		return nil, fmt.Errorf("этап 4 (xbox live token): пустой ответ от xboxlive")
	}
	uhs := xbl.DisplayClaims.Xui[0].Uhs

	var xsts xblResponse
	xstsReq := xstsRequest{RelyingParty: "rp://api.minecraftservices.com/", TokenType: "JWT"}
	xstsReq.Properties.SandboxID = "RETAIL"
	xstsReq.Properties.UserTokens = []string{xbl.Token}
	if err := c.postJSON(ctx, xstsAuthURL, xstsReq, &xsts); err != nil {
		return nil, fmt.Errorf("этап 5 (xsts token): %w", err)
	}
	if xsts.Token == "" {
		return nil, fmt.Errorf("этап 5 (xsts token): аккаунт заблокирован по возрасту/региону или без Xbox-профиля")
	}

	var mcLogin mcLoginResponse
	mcReq := mcLoginRequest{IdentityToken: fmt.Sprintf("XBL3.0 x=%s;%s", uhs, xsts.Token)}
	if err := c.postJSON(ctx, mcLoginURL, mcReq, &mcLogin); err != nil {
		return nil, fmt.Errorf("этап 6 (minecraft access token): %w", err)
	}
	if mcLogin.AccessToken == "" {
		return nil, fmt.Errorf("этап 6 (minecraft access token): пустой ответ от minecraftservices.com")
	}

	owns, err := c.ownsMinecraft(ctx, mcLogin.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("проверка владения лицензией: %w", err)
	}
	if !owns {
		return nil, fmt.Errorf("на этом аккаунте Microsoft не куплена лицензия Minecraft")
	}

	var profile mcProfileResponse
	if err := c.getJSON(ctx, mcProfileURL, mcLogin.AccessToken, &profile); err != nil {
		return nil, fmt.Errorf("этап 6 (профиль игрока): %w", err)
	}
	if profile.Name == "" {
		return nil, fmt.Errorf("не удалось получить никнейм: аккаунт не привязан к профилю Minecraft (Java Edition)")
	}

	return &Result{
		AccessToken:  mcLogin.AccessToken,
		RefreshToken: msTokens.RefreshToken,
		UUID:         formatUUID(profile.ID),
		Username:     profile.Name,
		XUID:         uhs,
	}, nil
}

func (c *Config) ownsMinecraft(ctx context.Context, mcAccessToken string) (bool, error) {
	var resp struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, mcEntitleURL, mcAccessToken, &resp); err != nil {
		return false, err
	}
	return len(resp.Items) > 0, nil
}

func formatUUID(raw string) string {
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != 32 {
		return raw
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:32])
}

func buildAuthURL(clientID, redirectURI string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", redirectURI)
	v.Set("scope", scope)
	v.Set("prompt", "select_account")
	return authorizeURL + "?" + v.Encode()
}

func (c *Config) postForm(ctx context.Context, endpoint string, form url.Values, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

func (c *Config) postJSON(ctx context.Context, endpoint string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-xbl-contract-version", "1")
	return c.do(req, out)
}

func (c *Config) getJSON(ctx context.Context, endpoint, bearer string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	return c.do(req, out)
}

func (c *Config) do(req *http.Request, out interface{}) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s: %s", strconv.Itoa(resp.StatusCode), string(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
