package trakt

import "time"

// Token represents OAuth token
type Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"` // Unix timestamp
}

// IsExpired checks if token is expired
func (t *Token) IsExpired() bool {
	createdTime := time.Unix(t.CreatedAt, 0)
	return time.Now().After(createdTime.Add(time.Duration(t.ExpiresIn) * time.Second))
}

// DeviceCodeResponse represents device code response
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}
