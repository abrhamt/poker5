package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeezSMSSenderPostsFormAndStripsPlus(t *testing.T) {
	var gotPhone, gotToken, gotMsg, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotToken, gotPhone, gotMsg = r.PostFormValue("token"), r.PostFormValue("phone"), r.PostFormValue("msg")
		gotType = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"error":false,"msg":"queued"}`))
	}))
	defer srv.Close()

	s := &GeezSMSSender{Token: "tok", Endpoint: srv.URL, Client: srv.Client()}
	if err := s.Send(context.Background(), "+251911223344", "Golden Poker: 123456 is your verification code."); err != nil {
		t.Fatalf("send: %v", err)
	}

	if gotToken != "tok" {
		t.Errorf("token = %q", gotToken)
	}
	// The gateway rejects the leading +, which is the form everything upstream
	// of the sender uses.
	if gotPhone != "251911223344" {
		t.Errorf("phone = %q, want 251911223344", gotPhone)
	}
	if !strings.Contains(gotMsg, "123456") {
		t.Errorf("msg = %q", gotMsg)
	}
	if !strings.HasPrefix(gotType, "application/x-www-form-urlencoded") {
		t.Errorf("content-type = %q", gotType)
	}
}

// The gateway reports refusals in the body with a 200 status, so a send that
// never happened must not read as success.
func TestGeezSMSSenderFailsOnErrorBody(t *testing.T) {
	for _, body := range []string{
		`{"error":true,"msg":"insufficient balance"}`,
		`{"error":"true","msg":"invalid token"}`,
		`not json at all`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		s := &GeezSMSSender{Token: "tok", Endpoint: srv.URL, Client: srv.Client()}
		if err := s.Send(context.Background(), "+251911223344", "msg"); err == nil {
			t.Errorf("body %q: want error, got nil", body)
		}
		srv.Close()
	}
}

func TestGeezSMSSenderFailsOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`unauthorized`))
	}))
	defer srv.Close()

	s := &GeezSMSSender{Token: "bad", Endpoint: srv.URL, Client: srv.Client()}
	if err := s.Send(context.Background(), "+251911223344", "msg"); err == nil {
		t.Fatal("want error on 401, got nil")
	}
}

func TestNewGeezSMSSenderNilWithoutToken(t *testing.T) {
	t.Setenv("GEEZSMS_TOKEN", "")
	if s := NewGeezSMSSender(); s != nil {
		t.Fatal("want nil sender when GEEZSMS_TOKEN is unset")
	}
	t.Setenv("GEEZSMS_TOKEN", "tok")
	s := NewGeezSMSSender()
	if s == nil || s.Endpoint != geezSMSEndpoint {
		t.Fatalf("sender = %+v", s)
	}
}
