package mocks

import (
	"errors"
)

var errUnauthorized = errors.New("Unauthorized")

// ManagerClient struct represents Manager client.
type ManagerClient struct {
	email string
}

// CanAccess mocks real manager client CanAccess method.
func (c ManagerClient) CanAccess(channel string, token string) (string, error) {
	if c.email == "" {
		return "", errUnauthorized
	}
	return c.email, nil
}

// NewManagerClient method creates new mocked manager service.
func NewManagerClient(email string) ManagerClient {
	return ManagerClient{email}
}
