package database

import (
	"strings"
	"testing"
	"time"
)

func TestPoolBoundsAndRedaction(t *testing.T) {
	c, e := PoolConfig("postgres://u:secret@127.0.0.1/test")
	if e != nil || c.MaxConns != 4 || c.ConnConfig.ConnectTimeout != 10*time.Second {
		t.Fatal(c, e)
	}
	c, e = PoolConfig("postgres://u:secret@127.0.0.1/test?pool_max_conns=1")
	if e != nil || c.MaxConns != 1 {
		t.Fatal(c, e)
	}
	for _, dsn := range []string{"postgres://u:secret@127.0.0.1/test?pool_max_conns=100", "postgres://u:secret@127.0.0.1/test?pool_max_conns=2&pool_min_conns=3", "postgres://u:secret@127.0.0.1/test?pool_max_conns=bad"} {
		_, e = PoolConfig(dsn)
		if e == nil || strings.Contains(e.Error(), "secret") {
			t.Fatal(e)
		}
	}
}
