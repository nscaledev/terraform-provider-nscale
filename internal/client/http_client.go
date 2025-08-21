package client

import (
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
)

type HTTPClient struct {
	internal     *http.Client
	userAgent    string
	serviceToken string
}

func NewHTTPClient(userAgent, serviceToken string) *HTTPClient {
	return &HTTPClient{
		internal:     retryablehttp.NewClient().StandardClient(),
		userAgent:    userAgent,
		serviceToken: serviceToken,
	}
}

func (c *HTTPClient) Do(r *http.Request) (*http.Response, error) {
	r.Header.Set("User-Agent", c.userAgent)
	r.Header.Set("Authorization", c.serviceToken)
	return c.internal.Do(r)
}
