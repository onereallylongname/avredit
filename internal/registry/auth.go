package registry

import "net/http"

// AuthType selects how the client authenticates to the registry.
type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthBasic  AuthType = "basic"
	AuthBearer AuthType = "bearer"
)

// authTransport wraps an underlying http.RoundTripper, adding credentials to
// every outgoing request. Credentials are held only in memory for the
// lifetime of the process (see internal/config for how they're sourced).
type authTransport struct {
	Type     AuthType
	Username string
	Password string // used with AuthBasic
	Token    string // used with AuthBearer
	Base     http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request before mutating headers, per http.RoundTripper contract.
	req = req.Clone(req.Context())
	switch t.Type {
	case AuthBasic:
		if t.Username != "" || t.Password != "" {
			req.SetBasicAuth(t.Username, t.Password)
		}
	case AuthBearer:
		if t.Token != "" {
			req.Header.Set("Authorization", "Bearer "+t.Token)
		}
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
