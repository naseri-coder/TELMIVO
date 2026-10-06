package config

import "testing"

func TestTrustedProxyConfigurationFailsClosed(t *testing.T) {
	env := productionEnv()
	for _, value := range []string{"0.0.0.0/0", "::/0", "invalid"} {
		env["TRUSTED_PROXY_CIDRS"] = value
		if _, err := Load(func(k string) string { return env[k] }); err == nil {
			t.Fatal("invalid trust accepted", value)
		}
	}
	env["TRUSTED_PROXY_CIDRS"] = "192.0.2.1/32,2001:db8::1/128"
	c, err := Load(func(k string) string { return env[k] })
	if err != nil || len(c.TrustedProxyCIDRs) != 2 {
		t.Fatal("explicit peers rejected", err)
	}
}
