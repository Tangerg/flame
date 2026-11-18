package main

import (
	"io"
	"net/http"
	"strings"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func upstream(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}

const issueJSON = `{"id":4200,"number":42,"title":"Inspected issue","body":"Original content","state":"open","labels":[],"assignees":[],"html_url":"https://github.com/owner/repo/issues/42","user":{"login":"author"},"updated_at":"2026-10-20T00:00:00Z"}`
