// LIC-A9 Phase51-J: production image release guard, standard library only.
// Standalone usage:
//
//	go list -json -deps ./cmd/... | go run ./scripts/release/phase51j_guard.go selected
//	go run ./scripts/release/phase51j_guard.go binary /out/api go1.27.2
//
// This is a verification tool, not an installation/runtime license bypass.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type sourcePackage struct {
	ImportPath string
	GoFiles    []string
	CgoFiles   []string
}

const (
	runtimeImport = "github.com/naseri-coder/TELMIVO/internal/platform/runtime"
	licenseImport = "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
)

func checkSelected(input io.Reader) error {
	decoder := json.NewDecoder(input)
	foundRuntime, foundLicense := false, false
	for {
		var pkg sourcePackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("invalid compiler package graph: %w", err)
		}
		if pkg.ImportPath != runtimeImport && pkg.ImportPath != licenseImport {
			continue
		}
		if pkg.ImportPath == runtimeImport {
			if foundRuntime {
				return errors.New("duplicate runtime package in release graph")
			}
			foundRuntime = true
		} else {
			if foundLicense {
				return errors.New("duplicate license package in release graph")
			}
			foundLicense = true
		}
		defaultStartup := false
		for _, name := range append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...) {
			if strings.Contains(name, "/") || strings.Contains(name, "\\") {
				return errors.New("unexpected source file path in package graph")
			}
			if name == "license_startup_default.go" && pkg.ImportPath == runtimeImport {
				defaultStartup = true
			}
			// If a developer accidentally adds a permissive build tag, the
			// real go list graph, not filename grep against the repository,
			// catches the actual compiled test-only source.
			if name == "fixture_integration.go" || name == "browser_test_policy.go" ||
				name == "license_startup_browser_fixture.go" ||
				strings.HasPrefix(name, "browser_test_") {
				return fmt.Errorf("test-only V2 signer/admission compiled into release: %s", name)
			}
		}
		if pkg.ImportPath == runtimeImport && !defaultStartup {
			return errors.New("trusted OpenEnvironment default startup missing")
		}
	}
	if !foundRuntime || !foundLicense {
		return errors.New("missing trusted Product license/runtime package in release graph")
	}
	return nil
}

func checkBuildInfo(info io.Reader, expectedGo string) error {
	if expectedGo == "" {
		return errors.New("missing pinned Go toolchain version")
	}
	scan := bufio.NewScanner(info)
	foundVersion, foundBuildMode, foundCGO := false, false, false
	for scan.Scan() {
		line := scan.Text()
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasSuffix(fields[0], ":") && fields[1] == expectedGo {
			foundVersion = true
		}
		if len(fields) >= 2 && fields[0] == "build" {
			attribute := fields[1]
			if strings.HasPrefix(attribute, "-tags=") {
				return fmt.Errorf("release binary was compiled with explicit build tags: %s", attribute)
			}
			if attribute == "-buildmode=exe" {
				foundBuildMode = true
			}
			if attribute == "CGO_ENABLED=0" {
				foundCGO = true
			}
		}
	}
	if err := scan.Err(); err != nil {
		return err
	}
	if !foundVersion || !foundBuildMode || !foundCGO {
		return errors.New("release binary does not prove pinned Go compiler, executable mode and disabled CGO")
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: phase51j_guard [selected|binary <path> <go-version>]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "selected":
		if len(os.Args) != 2 {
			err = errors.New("selected mode takes no extra arguments")
		} else {
			err = checkSelected(os.Stdin)
		}
	case "binary":
		if len(os.Args) != 4 {
			err = errors.New("binary mode needs immutable path and Go version")
		} else {
			var output []byte
			output, err = exec.Command("go", "version", "-m", os.Args[2]).Output()
			if err == nil {
				err = checkBuildInfo(strings.NewReader(string(output)), os.Args[3])
			}
		}
	default:
		err = errors.New("unknown release guard mode")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "LIC_A9_RELEASE_ARTIFACT_REJECTED:", err)
		os.Exit(1)
	}
	fmt.Println("LIC_A9_RELEASE_ARTIFACT_GUARD_PASS")
}
