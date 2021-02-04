package api

import (
	"net/http"
	"os"
	"time"

	"github.com/gorilla/securecookie"
)

const (
	// CookieName is the name of the cookie where we store the user's JWT token.
	CookieName = "skynet-jwt"
	// cookieValidity specifies for how long the cookie is valid. In seconds.
	cookieValidity = 604800 // one week

	// envCookieDomain holds the name of the environment variable for the
	// domain name of the portal
	envCookieDomain = "COOKIE_DOMAIN"
	// envCookieHashKey holds the name of the env var which holds the key we use
	// to hash cookies.
	envCookieHashKey = "COOKIE_HASH_KEY"
	// envCookieEncKey holds the name of the env var which holds the key we use
	// to encrypt cookies.
	envCookieEncKey = "COOKIE_ENC_KEY"
)

var (
	secureCookie = func() *securecookie.SecureCookie {
		var hashKey = []byte(os.Getenv(envCookieHashKey))
		var blockKey = []byte(os.Getenv(envCookieEncKey))
		return securecookie.New(hashKey, blockKey)
	}()
)

// writeCookie is a helper function that writes the given JWT token as a
// secure cookie.
func writeCookie(w http.ResponseWriter, token string, exp int64) error {
	if exp <= 0 || time.Unix(exp, 0).Before(time.Now().UTC()) {
		exp = time.Now().UTC().Unix()
	}
	encodedValue, err := secureCookie.Encode(CookieName, token)
	if err != nil {
		return err
	}
	// Allow this cookie to be used on all subdomains of this domain.
	domain, ok := os.LookupEnv(envCookieDomain)
	if !ok {
		domain = "127.0.0.1"
	}
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    encodedValue,
		HttpOnly: true,
		Path:     "/",
		Domain:   domain,
		Expires:  time.Unix(exp, 0),
		MaxAge:   cookieValidity,
		Secure:   true, // do not send over insecure channels, e.g. HTTP
		SameSite: 1,    // https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00
	}
	http.SetCookie(w, cookie)
	return nil
}
