package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	apple "github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
)

func TestR5BConsumerDecodeRejectsSecretFields(t *testing.T) {
	for _, body := range []string{
		`{"goal":"CREATE_ACCOUNT","channel":"WEB","locale":"fa-IR","region":"DE","idempotency_key":"r5b","password":"secret"}`,
		`{"goal":"CREATE_ACCOUNT","channel":"WEB","locale":"fa-IR","region":"DE","idempotency_key":"r5b","otp":"123456"}`,
		`{"goal":"CREATE_ACCOUNT","channel":"WEB","locale":"fa-IR","region":"DE","idempotency_key":"r5b","verification_code":"123456"}`,
	} {
		req := httptest.NewRequest("POST", "/api/v1/apple-services/consumer", strings.NewReader(body))
		rec := httptest.NewRecorder()
		var in apple.ConsumerStart
		if err := decode(rec, req, &in); err == nil {
			t.Fatalf("sensitive/unknown consumer field was accepted: %s", body)
		}
	}

	valid := `{"goal":"CREATE_ACCOUNT","channel":"WEB","locale":"fa-IR","region":"DE","idempotency_key":"r5b"}`
	req := httptest.NewRequest("POST", "/api/v1/apple-services/consumer", strings.NewReader(valid))
	rec := httptest.NewRecorder()
	var in apple.ConsumerStart
	if err := decode(rec, req, &in); err != nil {
		t.Fatal(err)
	}
}
