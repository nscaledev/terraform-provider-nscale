/*
Copyright 2025 Nscale

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	r.Header.Set("Authorization", "Bearer "+c.serviceToken)
	return c.internal.Do(r)
}
