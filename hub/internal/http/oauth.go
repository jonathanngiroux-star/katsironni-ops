package http

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- SIWE endpoints ----

type siweNonceReq struct{ Address string `json:"address" binding:"required"` }

func (s *Server) siweNonce(c *gin.Context) {
	var req siweNonceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address required"})
		return
	}
	_, message, err := s.auth.CreateSIWENonce(req.Address)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}

type siweVerifyReq struct {
	Address   string `json:"address" binding:"required"`
	Message   string `json:"message" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

func (s *Server) siweVerify(c *gin.Context) {
	var req siweVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address, message and signature required"})
		return
	}
	wallet, err := s.auth.VerifySIWE(req.Address, req.Message, req.Signature)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	uid, err := s.auth.FindOrCreateSocialUser("siwe", wallet, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "account lookup failed"})
		return
	}
	token, err := s.auth.CreateSession(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session failed"})
		return
	}
	c.SetCookie("hub_session", token, int((30*24*time.Hour).Seconds()), "/", ".katsironni.com", true, true)
	c.JSON(http.StatusOK, gin.H{"ok": true, "redirect": "/account"})
}

// ---- OAuth providers (Google, X, GitHub) ----

type oauthProvider struct {
	Key          string
	AuthURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scope        string
	UsePKCE      bool
}

func (s *Server) oauthProvider(key string) (*oauthProvider, bool) {
	var p *oauthProvider
	switch key {
	case "google":
		p = &oauthProvider{Key: "google",
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			ClientID: s.cfg.GoogleClientID, ClientSecret: s.cfg.GoogleClientSecret,
			Scope: "openid email profile"}
	case "x":
		p = &oauthProvider{Key: "x",
			AuthURL:  "https://twitter.com/i/oauth2/authorize",
			TokenURL: "https://api.x.com/2/oauth2/token",
			ClientID: s.cfg.XClientID, ClientSecret: s.cfg.XClientSecret,
			Scope: "users.read tweet.read", UsePKCE: true}
	case "github":
		p = &oauthProvider{Key: "github",
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
			ClientID: s.cfg.GitHubClientID, ClientSecret: s.cfg.GitHubClientSecret,
			Scope: "read:user user:email"}
	default:
		return nil, false
	}
	if p.ClientID == "" || p.ClientSecret == "" {
		return nil, false
	}
	return p, true
}

// socialEnabled reports which buttons to render (SIWE always on).
func (s *Server) socialEnabled() map[string]bool {
	m := map[string]bool{"siwe": true}
	for _, k := range []string{"google", "x", "github"} {
		if _, ok := s.oauthProvider(k); ok {
			m[k] = true
		}
	}
	return m
}

func randToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var providerNames = map[string]string{
	"google": "Google",
	"x":      "X",
	"github": "GitHub",
}

func notConfiguredRedirect(c *gin.Context, key string) {
	name := providerNames[key]
	if name == "" {
		name = key
	}
	c.Redirect(http.StatusFound, "/login?notice="+url.QueryEscape(name+" sign-in coming soon — check back shortly. Email and MetaMask work today!"))
}

func (s *Server) oauthStart(c *gin.Context) {
	key := strings.TrimPrefix(c.Request.URL.Path, "/auth/")
	key = strings.TrimSuffix(key, "/start")
	p, ok := s.oauthProvider(key)
	if !ok {
		notConfiguredRedirect(c, key)
		return
	}
	state := randToken(16)
	v := url.Values{}
	v.Set("client_id", p.ClientID)
	v.Set("redirect_uri", s.cfg.PublicURL+"/auth/"+key+"/callback")
	v.Set("response_type", "code")
	v.Set("scope", p.Scope)
	v.Set("state", state)
	if p.UsePKCE {
		verifier := randToken(32)
		sum := sha256.Sum256([]byte(verifier))
		v.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
		v.Set("code_challenge_method", "S256")
		c.SetCookie("oauth_verifier", verifier, 600, "/", ".katsironni.com", true, true)
	}
	c.SetCookie("oauth_state", state, 600, "/", ".katsironni.com", true, true)
	c.Redirect(http.StatusFound, p.AuthURL+"?"+v.Encode())
}

func (s *Server) oauthCallback(c *gin.Context) {
	key := strings.TrimPrefix(c.Request.URL.Path, "/auth/")
	key = strings.TrimSuffix(key, "/callback")
	p, ok := s.oauthProvider(key)
	if !ok {
		notConfiguredRedirect(c, key)
		return
	}
	// state check
	qState := c.Query("state")
	ckState, err := c.Cookie("oauth_state")
	if err != nil || qState == "" || ckState != qState {
		c.Redirect(http.StatusFound, "/login?error=invalid+oauth+state")
		return
	}
	c.SetCookie("oauth_state", "", -1, "/", ".katsironni.com", true, true)
	if codeErr := c.Query("error"); codeErr != "" {
		c.Redirect(http.StatusFound, "/login?error="+url.QueryEscape(codeErr))
		return
	}
	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/login?error=missing+code")
		return
	}

	// exchange code for token
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.cfg.PublicURL+"/auth/"+key+"/callback")
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	if p.UsePKCE {
		verifier, verr := c.Cookie("oauth_verifier")
		if verr != nil {
			c.Redirect(http.StatusFound, "/login?error=missing+verifier")
			return
		}
		form.Set("code_verifier", verifier)
		c.SetCookie("oauth_verifier", "", -1, "/", ".katsironni.com", true, true)
	}
	accessToken, exErr := s.oauthTokenExchange(p.TokenURL, form)
	if exErr != nil {
		c.Redirect(http.StatusFound, "/login?error="+url.QueryEscape(exErr.Error()))
		return
	}

	// provider identity
	providerID, email, idErr := s.oauthIdentity(key, accessToken)
	if idErr != nil {
		c.Redirect(http.StatusFound, "/login?error="+url.QueryEscape(idErr.Error()))
		return
	}

	uid, err := s.auth.FindOrCreateSocialUser(key, providerID, email)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=account+link+failed")
		return
	}
	token, err := s.auth.CreateSession(uid)
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error=session+failed")
		return
	}
	c.SetCookie("hub_session", token, int((30*24*time.Hour).Seconds()), "/", ".katsironni.com", true, true)
	c.Redirect(http.StatusFound, "/account")
}

func (s *Server) oauthTokenExchange(tokenURL string, form url.Values) (string, error) {
	req, _ := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("token exchange failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if jerr := json.Unmarshal(body, &tr); jerr != nil || tr.AccessToken == "" {
		if tr.Error != "" {
			return "", errors.New(tr.Error)
		}
		return "", fmt.Errorf("no access token (HTTP %d)", resp.StatusCode)
	}
	return tr.AccessToken, nil
}

func (s *Server) oauthIdentity(key, accessToken string) (providerID, email string, err error) {
	switch key {
	case "google":
		var u struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
		}
		if err = fetchJSON("https://openidconnect.googleapis.com/v1/userinfo", accessToken, &u); err != nil {
			return "", "", err
		}
		return u.Sub, u.Email, nil
	case "github":
		var u struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Email *string `json:"email"`
		}
		if err = fetchJSON("https://api.github.com/user", accessToken, &u); err != nil {
			return "", "", err
		}
		email = ""
		if u.Email != nil {
			email = *u.Email
		}
		if email == "" {
			// primary verified email via /user/emails
			var emails []struct {
				Email   string `json:"email"`
				Primary bool   `json:"primary"`
			}
			if err = fetchJSON("https://api.github.com/user/emails", accessToken, &emails); err == nil {
				for _, e := range emails {
					if e.Primary {
						email = e.Email
						break
					}
				}
			}
		}
		return fmt.Sprintf("%d", u.ID), email, nil
	case "x":
		var r struct {
			Data struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Email    string `json:"email"`
			} `json:"data"`
		}
		if err = fetchJSON("https://api.x.com/2/users/me?user.fields=id,username,email", accessToken, &r); err != nil {
			return "", "", err
		}
		return r.Data.ID, r.Data.Email, nil
	}
	return "", "", errors.New("unknown provider")
}

func fetchJSON(url, bearer string, out any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "katsironni-hub")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("provider request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if jerr := json.Unmarshal(body, out); jerr != nil {
		return errors.New("provider response parse failed")
	}
	return nil
}
