/*
Copyright 2018 The Knative Authors
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

package gatewayserver

import (
	"crypto/tls"
	"net"
	"net/http"

	"golang.org/x/net/http2"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

// NewHttpTransport will use the appropriate transport for the request's HTTP protocol version
func newHTTPTransport(v1 http.RoundTripper, v2 http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		t := v1
		if r.ProtoMajor == 2 {
			t = v2
		}

		return t.RoundTrip(r)
	})
}

var http2Transport http.RoundTripper = &http2.Transport{
	AllowHTTP: true,
	DialTLS: func(netw, addr string, cfg *tls.Config) (net.Conn, error) {
		return net.Dial(netw, addr)
	},
}

// AutoTransport uses h2c for HTTP2 requests and falls back to `http.DefaultTransport` for all others
var autoTransport = newHTTPTransport(http.DefaultTransport, http2Transport)
