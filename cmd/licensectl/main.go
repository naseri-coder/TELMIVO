// licensectl accepts activation codes through stdin only, never process argv.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	l "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func run() error {
	m, c, e := l.OpenEnvironment()
	if e != nil {
		return e
	}
	if len(os.Args) < 2 {
		return fmt.Errorf("LICENSE_COMMAND_REQUIRED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch os.Args[1] {
	case "identity":
		id, key, e := m.PublicIdentity()
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"installation_id": id, "installation_public_key": base64.RawURLEncoding.EncodeToString(key)})
	case "status":
		return json.NewEncoder(os.Stdout).Encode(m.Decision())
	case "authority-status":
		return c.Status(ctx)
	case "refresh":
		return c.Refresh(ctx)
	case "deactivate":
		return c.Deactivate(ctx)
	case "activate":
		if len(os.Args) != 4 {
			return fmt.Errorf("LICENSE_ID_AND_EPOCH_REQUIRED")
		}
		epoch, e := strconv.ParseInt(os.Args[3], 10, 64)
		if e != nil {
			return fmt.Errorf("LICENSE_EPOCH_INVALID")
		}
		b, e := io.ReadAll(io.LimitReader(os.Stdin, 514))
		if e != nil || len(b) > 513 {
			return fmt.Errorf("ACTIVATION_INPUT_INVALID")
		}
		return c.Activate(ctx, strings.TrimSpace(string(b)), os.Args[2], epoch)
	default:
		return fmt.Errorf("LICENSE_COMMAND_INVALID")
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "LICENSE_COMMAND_FAILED")
		os.Exit(1)
	}
}
