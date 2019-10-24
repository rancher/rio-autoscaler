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
	"strconv"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	requestCountHTTPHeader = "Request-Retry-Count"
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

type retryCond func(*http.Response) bool

// RetryStatus will filter responses matching `status`
func retryStatus(status int) retryCond {
	return func(resp *http.Response) bool {
		return resp.StatusCode == status
	}
}

type retryRoundTripper struct {
	transport       http.RoundTripper
	backoffSettings wait.Backoff
	retryConditions []retryCond
}

// RetryRoundTripper retries a request on error or retry condition, using the given `retry` strategy
func newRetryRoundTripper(rt http.RoundTripper, b wait.Backoff, conditions ...retryCond) http.RoundTripper {
	return &retryRoundTripper{
		transport:       rt,
		backoffSettings: b,
		retryConditions: conditions,
	}
}

func (rrt *retryRoundTripper) RoundTrip(r *http.Request) (resp *http.Response, err error) {
	// The request body cannot be read multiple times for retries.
	// The workaround is to clone the request body into a byte reader
	// so the body can be read multiple times.
	if r.Body != nil {
		logrus.Debugf("Wrapping body in a rewinder.")
		r.Body = newRewinder(r.Body)
	}

	attempts := 0
	wait.ExponentialBackoff(rrt.backoffSettings, func() (bool, error) {
		attempts++
		r.Header.Add(requestCountHTTPHeader, strconv.Itoa(attempts))
		resp, err = rrt.transport.RoundTrip(r)

		if err != nil {
			logrus.Errorf("Error making a request: %s", err)
			return false, nil
		}

		for _, retryCond := range rrt.retryConditions {
			if retryCond(resp) {
				resp.Body.Close()
				return false, nil
			}
		}
		return true, nil
	})

	if err == nil {
		logrus.Infof("Finished after %d attempt(s). Response code: %d", attempts, resp.StatusCode)

		if resp.Header == nil {
			resp.Header = make(http.Header)
		}

		resp.Header.Add(requestCountHTTPHeader, strconv.Itoa(attempts))
	} else {
		logrus.Errorf("Failed after %d attempts. Last error: %v", attempts, err)
	}

	return
}
