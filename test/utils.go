package test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/NebulousLabs/skynet-accounts/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.sia.tech/siad/crypto"
)

const (
	// FauxEmailURI is a valid URI for sending emails that points to a local
	// mailslurper instance. That instance is most probably not running, so
	// trying to send mails with it will fail, but it's useful for testing with
	// the DependencySkipSendingEmails.
	FauxEmailURI = "smtps://test:test1@mailslurper:1025/?skip_ssl_verify=true"

	// UserSubLen is string length of a user's `sub` field
	UserSubLen = 36
)

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "17017",
	}
}

// CreateUser is a helper method which simplifies the creation of test users
func CreateUser(t *testing.T, at *AccountsTester, customEmail, customPassword string) (*database.User, func(user *database.User), error) {
	email := customEmail
	if email == "" {
		// Use the test's name as an email-compatible identifier.
		email = strings.ReplaceAll(t.Name(), "/", "_") + "@siasky.net"
	}
	password := customPassword
	if password == "" {
		password = hex.EncodeToString(fastrand.Bytes(16))
	}
	params := map[string]string{
		"email":    email,
		"password": password,
	}
	// Create a user.
	_, _, err := at.Post("/user", nil, params)
	if err != nil {
		return nil, nil, errors.AddContext(err, "user creation failed")
	}
	// Fetch the user from the DB, so we can delete it later.
	u, err := at.DB.UserByEmail(at.Ctx, email, false)
	if err != nil {
		return nil, nil, errors.AddContext(err, "failed to fetch user from the DB")
	}
	cleanup := func(user *database.User) {
		err = at.DB.UserDelete(at.Ctx, user)
		if err != nil {
			t.Errorf("Error while cleaning up user: %s", err.Error())
			return
		}
	}
	return u, cleanup, nil
}

// CreateUserAndLogin is a helper method that creates anew test user and
// immediately logs in with it, returning the user, the login cookie, a cleanup
// function that deletes the user.
func CreateUserAndLogin(t *testing.T, at *AccountsTester) (*database.User, *http.Cookie, func(user *database.User), error) {
	// Use the test's name as an email-compatible identifier.
	name := strings.ReplaceAll(t.Name(), "/", "_")
	params := map[string]string{
		"email":    name + "@siasky.net",
		"password": hex.EncodeToString(fastrand.Bytes(16)),
	}
	// Create a user.
	u, cleanup, err := CreateUser(t, at, params["email"], params["password"])
	if err != nil {
		return nil, nil, nil, err
	}
	// Log in with that user in order to make sure it exists.
	r, _, err := at.Post("/login", nil, params)
	if err != nil {
		return nil, nil, nil, err
	}
	// Grab the Skynet cookie, so we can make authenticated calls.
	c := ExtractCookie(r)
	if c == nil {
		return nil, nil, nil, err
	}
	return u, c, cleanup, nil
}

// CreateTestUpload creates a new skyfile and uploads it under the given user's
// account. Returns the skylink, the upload's id and error.
func CreateTestUpload(ctx context.Context, db *database.DB, user *database.User, size int64) (*database.Skylink, primitive.ObjectID, error) {
	// Create a skylink record for which to register an upload
	sl := RandomSkylink()
	skylink, err := db.Skylink(ctx, sl)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to create a test skylink")
	}
	err = db.SkylinkUpdate(ctx, skylink.ID, "test skylink "+sl, size)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to update skylink")
	}
	// Get the updated skylink.
	skylink, err = db.Skylink(ctx, sl)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to fetch skylink from DB")
	}
	if skylink.Size != size {
		return nil, primitive.ObjectID{}, errors.AddContext(err, fmt.Sprintf("expected skylink size to be %d, got %d.", size, skylink.Size))
	}
	// Register an upload.
	return RegisterTestUpload(ctx, db, user, skylink)
}

// RandomSkylink generates a random skylink
func RandomSkylink() string {
	var h crypto.Hash
	fastrand.Read(h[:])
	sl, _ := skymodules.NewSkylinkV1(h, 0, 0)
	return sl.String()
}

// RegisterTestUpload registers an upload of the given skylink by the given user.
// Returns the skylink, the upload's id and error.
func RegisterTestUpload(ctx context.Context, db *database.DB, user *database.User, skylink *database.Skylink) (*database.Skylink, primitive.ObjectID, error) {
	up, err := db.UploadCreate(ctx, *user, *skylink)
	if err != nil {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "failed to register an upload")
	}
	if up.UserID != user.ID {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "expected upload's userId to match the uploader's id")
	}
	if up.SkylinkID != skylink.ID {
		return nil, primitive.ObjectID{}, errors.AddContext(err, "expected upload's skylinkId to match the given skylink's id")
	}
	return skylink, up.ID, nil
}
