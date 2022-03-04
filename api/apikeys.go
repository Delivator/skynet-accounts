package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/SkynetLabs/skynet-accounts/database"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type (
	// APIKeyPOST describes the body of a POST request that creates an API key
	APIKeyPOST struct {
		Public   bool     `json:"public,string"`
		Skylinks []string `json:"skylinks"`
	}
	// APIKeyPUT describes the request body for updating an API key
	APIKeyPUT struct {
		Skylinks []string
	}
	// APIKeyPATCH describes the request body for updating an API key by
	// providing only the requested changes
	APIKeyPATCH struct {
		Add    []string
		Remove []string
	}
	// APIKeyResponse is an API DTO which mirrors database.APIKey.
	// TODO Should we reveal the Key each time for public keys?
	APIKeyResponse struct {
		ID        primitive.ObjectID `json:"id"`
		UserID    primitive.ObjectID `json:"-"`
		Public    bool               `json:"public,string"`
		Key       database.APIKey    `json:"-"`
		Skylinks  []string           `json:"skylinks"`
		CreatedAt time.Time          `json:"createdAt"`
	}
	// APIKeyResponseWithKey is an API DTO which mirrors database.APIKey but
	// also reveals the value of the Key field. This should only be used on key
	// creation.
	// TODO Should we reveal the Key each time for public keys?
	APIKeyResponseWithKey struct {
		APIKeyResponse
		Key database.APIKey `json:"key"`
	}
)

// Valid checks if the request and its parts are valid.
func (akp APIKeyPOST) Valid() bool {
	if !akp.Public && len(akp.Skylinks) > 0 {
		return false
	}
	for _, s := range akp.Skylinks {
		if !database.ValidSkylinkHash(s) {
			return false
		}
	}
	return true
}

// FromAPIKey populates the struct's fields from the given API key.
// TODO This might be more convenient as a constructor.
func (rwk *APIKeyResponse) FromAPIKey(ak database.APIKeyRecord) {
	rwk.ID = ak.ID
	rwk.UserID = ak.UserID
	rwk.Public = ak.Public
	rwk.Key = ak.Key
	rwk.Skylinks = ak.Skylinks
	rwk.CreatedAt = ak.CreatedAt
}

// FromAPIKey populates the struct's fields from the given API key.
// TODO This might be more convenient as a constructor.
func (rwk *APIKeyResponseWithKey) FromAPIKey(ak database.APIKeyRecord) {
	rwk.ID = ak.ID
	rwk.UserID = ak.UserID
	rwk.Public = ak.Public
	rwk.Key = ak.Key
	rwk.Skylinks = ak.Skylinks
	rwk.CreatedAt = ak.CreatedAt
}

// userAPIKeyPOST creates a new API key for the user.
func (api *API) userAPIKeyPOST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body APIKeyPOST
	err := parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ak, err := api.staticDB.APIKeyCreate(req.Context(), *u, body.Public, body.Skylinks)
	if errors.Contains(err, database.ErrMaxNumAPIKeysExceeded) {
		err = errors.AddContext(err, "the maximum number of API keys a user can create is "+strconv.Itoa(database.MaxNumAPIKeysPerUser))
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	var resp APIKeyResponseWithKey
	resp.FromAPIKey(*ak)
	api.WriteJSON(w, resp)
}

// userAPIKeyGET returns a single API key.
func (api *API) userAPIKeyGET(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	ak, err := api.staticDB.APIKeyGet(req.Context(), akID)
	// If there is no such API key or it doesn't exist, return a 404.
	if errors.Contains(err, mongo.ErrNoDocuments) || (err == nil && ak.UserID != u.ID) {
		api.WriteError(w, nil, http.StatusNotFound)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	var resp APIKeyResponse
	resp.FromAPIKey(ak)
	api.WriteJSON(w, resp)
}

// userAPIKeyLIST lists all API keys associated with the user.
func (api *API) userAPIKeyLIST(u *database.User, w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	aks, err := api.staticDB.APIKeyList(req.Context(), *u)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]APIKeyResponse, 0, len(aks))
	for _, ak := range aks {
		var r APIKeyResponse
		r.FromAPIKey(ak)
		resp = append(resp, r)
	}
	api.WriteJSON(w, resp)
}

// userAPIKeyDELETE removes an API key.
func (api *API) userAPIKeyDELETE(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyDelete(req.Context(), *u, akID)
	if err == mongo.ErrNoDocuments {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userAPIKeyPUT updates an API key. Only possible for public API keys.
func (api *API) userAPIKeyPUT(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	var body APIKeyPUT
	err = parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyUpdate(req.Context(), *u, akID, body.Skylinks)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// userAPIKeyPATCH patches an API key. The difference between PUT and PATCH is
// that PATCH only specifies the changes while PUT provides the expected list of
// covered skylinks. Only possible for public API keys.
func (api *API) userAPIKeyPATCH(u *database.User, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	akID, err := primitive.ObjectIDFromHex(ps.ByName("id"))
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	var body APIKeyPATCH
	err = parseRequestBodyJSON(req.Body, LimitBodySizeLarge, &body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	err = api.staticDB.APIKeyPatch(req.Context(), *u, akID, body.Add, body.Remove)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}
