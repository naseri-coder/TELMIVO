package mainbot

import (
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
	"testing"
)

func TestCustomerRoutes(t *testing.T) {
	for key, path := range map[string]string{"dashboard": "/", "channels": "/channels", "groups": "/groups", "bots": "/customer-bots", "tickets": "/support", "wallet": "/wallet", "plans": "/plans", "settings": "/settings", "admin": "/admin/policies"} {
		actual, ok := Route(localization.Text("menu." + key))
		if !ok || actual != path {
			t.Fatal(key, actual)
		}
	}
	if path, ok := Route("/start campaign"); !ok || path != "/" {
		t.Fatal("start")
	}
	if _, ok := Route("unknown"); ok {
		t.Fatal("unknown accepted")
	}
}
