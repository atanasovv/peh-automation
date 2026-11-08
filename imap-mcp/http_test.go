package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestHTTPAccessAndDiscovery(t *testing.T) {
	handler, err := mailHTTPHandler(newMailServer(mailbox{}), testToken, "mail.example.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/mcp"} {
		for _, test := range []struct {
			name, host, origin, authorization string
			status                            int
		}{
			{"missing token", "mail.example.com", "", "", http.StatusUnauthorized},
			{"wrong token", "mail.example.com", "", "Bearer wrong", http.StatusUnauthorized},
			{"wrong host", "other.example.com", "", "Bearer " + testToken, http.StatusForbidden},
			{"wrong origin", "mail.example.com", "https://other.example.com", "Bearer " + testToken, http.StatusForbidden},
			{"valid request", "mail.example.com", "", "Bearer " + testToken, http.StatusOK},
			{"same origin", "mail.example.com", "https://mail.example.com", "Bearer " + testToken, http.StatusOK},
		} {
			t.Run(path+test.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "https://"+test.host+path,
					strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
				req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey,
					&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				if test.authorization != "" {
					req.Header.Set("Authorization", test.authorization)
				}
				if test.origin != "" {
					req.Header.Set("Origin", test.origin)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code != test.status {
					t.Fatalf("status %d, want %d: %s", response.Code, test.status, response.Body)
				}
				if test.status == http.StatusOK {
					var result struct {
						Result struct {
							Tools []struct {
								Name string `json:"name"`
							} `json:"tools"`
						} `json:"result"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					names := map[string]bool{}
					for _, tool := range result.Result.Tools {
						names[tool.Name] = true
					}
					if len(names) != 3 || !names["search_mail"] || !names["read_mail"] || !names["save_reply_draft"] {
						t.Fatalf("unexpected tools: %v", names)
					}
				}
			})
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatal("health probe failed")
	}
}

func TestHTTPRejectsIncompleteConfiguration(t *testing.T) {
	for _, host := range []string{"", "example.com/path", "example.com?query", "user@example.com"} {
		if _, err := mailHTTPHandler(newMailServer(mailbox{}), testToken, host); err == nil {
			t.Fatalf("accepted invalid public host %q", host)
		}
	}
	if _, err := mailHTTPHandler(newMailServer(mailbox{}), "short", "mail.example.com"); err == nil {
		t.Fatal("accepted short token")
	}
}
