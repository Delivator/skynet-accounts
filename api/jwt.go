package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/lestrrat/go-jwx/jwk"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// oathkeeperPubKeys is the public RS key exposed by Oathkeeper for JWT
	// validation. It's available at oathkeeperPubKeyURL.
	oathkeeperPubKeys *jwk.Set = nil

	// oathkeeperPubKeyURL is the URL on which we can find the public key.
	oathkeeperPubKeyURL = "http://oathkeeper:4456/.well-known/jwks.json"
)

// ValidateToken verifies the validity of a JWT token, both in terms of validity
// of the signature and expiration time.
//
// Example token:
//
// Header:
//
//{
//  "alg": "RS256",
//  "kid": "a2aa9739-d753-4a0d-87ee-61f101050277",
//  "typ": "JWT"
//}
//
// Payload:
//
//{
//  "exp": 1607594172,
//  "iat": 1607593272,
//  "iss": "https://siasky.xyz/",
//  "jti": "1e5872ae-71d8-49ec-a550-4fc6163cbbf2",
//  "nbf": 1607593272,
//  "session": {
//    "active": true,
//    "authenticated_at": "2020-12-09T16:09:35.004003Z",
//    "expires_at": "2020-12-10T16:09:35.004003Z",
//    "id": "9911ad26-e47f-4ec4-86a1-fbbc7fd5073e",
//    "identity": {
//      "id": "695725d4-a345-4e68-919a-7395cb68484c",
//      "recovery_addresses": [
//        {
//          "id": "e2d847e1-1885-4edf-bccb-64b527b30096",
//          "value": "ivaylo@nebulous.tech",
//          "via": "email"
//        }
//      ],
//      "schema_id": "default",
//      "schema_url": "https://siasky.xyz/secure/.ory/kratos/public/schemas/default",
//      "traits": {
//        "email": "ivaylo@nebulous.tech",
//        "name": {
//          "first": "Ivaylo",
//          "last": "Novakov"
//        }
//      },
//      "verifiable_addresses": [
//        {
//          "id": "953b0c1a-def9-4fa2-af23-fb36c00768d2",
//          "status": "pending",
//          "value": "ivaylo@nebulous.tech",
//          "verified": false,
//          "verified_at": null,
//          "via": "email"
//        }
//      ]
//    },
//    "issued_at": "2020-12-09T16:09:35.004042Z"
//  },
//  "sub": "695725d4-a345-4e68-919a-7395cb68484c"
//}
func ValidateToken(t string) (*jwt.Token, error) {
	token, err := jwt.Parse(t, keyForToken)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token is invalid")
	}
	// TODO Verify issuer, scope, etc.?
	return token, nil
}

// keyForToken finds a suitable key for validating the
// given token among the public keys provided by Oathkeeper.
func keyForToken(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, errors.New(fmt.Sprintf("unexpected signing method: %v", token.Header["alg"]))
	}
	keySet, err := oathkeeperPublicKeys()
	if err != nil {
		return nil, err
	}
	if reflect.ValueOf(token.Header["kid"]).Kind() != reflect.String {
		return nil, errors.New("invalid jwk header - the kid field is not a string")
	}
	keys := keySet.LookupKeyID(token.Header["kid"].(string))
	if len(keys) == 0 {
		return nil, errors.New("no suitable keys found")
	}
	return keys[0].Materialize()
}

// oathkeeperPublicKeys checks whether we have the
// needed public key cached and if we don't it fetches it and caches it for us.
//
// See https://tools.ietf.org/html/rfc7517
// See https://auth0.com/blog/navigating-rs256-and-jwks/
// See http://self-issued.info/docs/draft-ietf-oauth-json-web-token.html
// Encoding RSA pub key: https://play.golang.org/p/mLpOxS-5Fy
func oathkeeperPublicKeys() (*jwk.Set, error) {
	if oathkeeperPubKeys == nil {
		fmt.Println("fetching JWKS from oathkeeper")
		r, err := http.Get(oathkeeperPubKeyURL) // #nosec G107: Potential HTTP request made with variable url
		if err != nil {
			fmt.Println("ERROR while fetching JWKS from oathkeeper", err)
		    return nil, err
		}
		defer r.Body.Close()
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Println("ERROR while reading JWKS from oathkeeper", err)
		    return nil, err
		}
		set, err := jwk.ParseString(string(b))
		if err != nil {
			fmt.Println("ERROR while parsing JWKS from oathkeeper", err)
			return nil, err
		}
		oathkeeperPubKeys = set
	}
	return oathkeeperPubKeys, nil
}

// tokenFromRequest extracts the JWT token from the request and returns it.
// Returns an empty string if there is no token.
func tokenFromRequest(r *http.Request) (string, error) {
	// Check the headers for a token.
	authHeader := r.Header.Get("Authorization")
	parts := strings.Split(authHeader, "Bearer")
	if len(parts) != 2 {
		return "", errors.New("invalid authorization header")
	}
	return strings.TrimSpace(parts[1]), nil
}

// tokenFromContext extracts the JWT token from the
// context and returns the contained user sub, claims and the token itself.
// The sub is the user id used in Kratos.
//
// Example claims structure:
//
// map[
//    exp:1.607594172e+09
//    iat:1.607593272e+09
//    iss:https://siasky.xyz/
//    jti:1e5872ae-71d8-49ec-a550-4fc6163cbbf2
//    nbf:1.607593272e+09
//    sub:695725d4-a345-4e68-919a-7395cb68484c
//    session:map[
//        active:true
//        authenticated_at:2020-12-09T16:09:35.004003Z
//        issued_at:2020-12-09T16:09:35.004042Z
//        expires_at:2020-12-10T16:09:35.004003Z
//        id:9911ad26-e47f-4ec4-86a1-fbbc7fd5073e
//        identity:map[
//            id:695725d4-a345-4e68-919a-7395cb68484c
//            recovery_addresses:[
//                map[
//                    id:e2d847e1-1885-4edf-bccb-64b527b30096
//                    value:ivaylo@nebulous.tech
//                    via:email
//                ]
//            ]
//            schema_id:default
//            schema_url:https://siasky.xyz/secure/.ory/kratos/public/schemas/default
//            traits:map[
//                email:ivaylo@nebulous.tech
//                name:map[
//                    first:Ivaylo
//                    last:Novakov
//                ]
//            ]
//            verifiable_addresses:[
//                map[
//                    id:953b0c1a-def9-4fa2-af23-fb36c00768d2
//                    status:pending
//                    value:ivaylo@nebulous.tech
//                    verified:true
//                    verified_at:2020-12-09T16:09:35.004042Z
//                    via:email
//                ]
//            ]
//        ]
//    ]
// ]
func tokenFromContext(req *http.Request) (sub string, claims jwt.MapClaims, token *jwt.Token, err error) {
	t, ok := req.Context().Value(ctxValue("token")).(*jwt.Token)
	if !ok {
		err = errors.New("failed to get token")
		return
	}
	if reflect.ValueOf(t.Claims).Kind() != reflect.ValueOf(jwt.MapClaims{}).Kind() {
		err = errors.New("the token does not contain the claims we expect")
		return
	}
	claims = t.Claims.(jwt.MapClaims)
	if reflect.ValueOf(claims["sub"]).Kind() != reflect.String {
		err = errors.New("the token does not contain the sub we expect")
	}
	subEntry, ok := claims["sub"]
	if !ok {
		claims = nil
		err = errors.New("jwt claims don't contain a valid sub")
		return
	}
	sub = subEntry.(string)
	token = t
	return
}

// TokenExpiration extracts and returns the `exp` claim of the given token.
// NOTE: It does NOT validate the token!
func TokenExpiration(t *jwt.Token) (int64, error) {
	if t == nil {
		return 0, errors.New("invalid token")
	}
	if reflect.ValueOf(t.Claims).Kind() != reflect.ValueOf(jwt.MapClaims{}).Kind() {
		return 0, errors.New("the token does not contain the claims we expect")
	}
	claims := t.Claims.(jwt.MapClaims)
	if reflect.ValueOf(claims["exp"]).Kind() != reflect.Float64 {
		return 0, errors.New("the token does not contain the claims we expect")
	}
	return int64(claims["exp"].(float64)), nil
}
