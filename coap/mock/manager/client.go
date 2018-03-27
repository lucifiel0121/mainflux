package mock

import (
	"errors"
)

var errUnauthorized = errors.New("Unauthorized")

// Client struct represents Manager client.
type Client struct {
	Email string
}

// CanAccess mocks real manager client CanAccess method.
func (c Client) CanAccess(channel string, token string) (string, error) {
	if c.Email == "" {
		return "", errUnauthorized
	}
	return c.Email, nil
}

// New method creates new mocked manager service.
func New(email string) Client {
	return Client{
		Email: email,
	}
}
