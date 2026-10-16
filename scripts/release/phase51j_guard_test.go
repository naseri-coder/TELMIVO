package main

import (
	"strings"
	"testing"
)

func samplePackageGraph(runtimeFiles, licenseFiles string) string {
	return `{"ImportPath":"github.com/example/irrelevant","GoFiles":["main.go"]}
{"ImportPath":"` + runtimeImport + `","GoFiles":` + runtimeFiles + `}
{"ImportPath":"` + licenseImport + `","GoFiles":` + licenseFiles + `}
`
}

func TestLICA9Phase51JRejectTestOnlyGoFiles(t *testing.T) {
	valid := samplePackageGraph(`["runtime.go","license_startup_default.go"]`, `["state.go","runtime.go"]`)
	if err := checkSelected(strings.NewReader(valid)); err != nil {
		t.Fatalf("ordinary Product graph was rejected: %v", err)
	}
	cases := []struct {
		name, graph string
	}{
		{"browser_startup", samplePackageGraph(`["runtime.go","license_startup_browser_fixture.go"]`, `["state.go"]`)},
		{"browser_policy", samplePackageGraph(`["runtime.go","license_startup_default.go"]`, `["state.go","browser_test_policy.go"]`)},
		{"signed_V2_test_fixture", samplePackageGraph(`["runtime.go","license_startup_default.go"]`, `["state.go","fixture_integration.go"]`)},
		{"missing_default", samplePackageGraph(`["runtime.go"]`, `["state.go"]`)},
		{"missing_license", `{"ImportPath":"` + runtimeImport + `","GoFiles":["runtime.go","license_startup_default.go"]}`},
		{"duplicate_runtime", valid + `{"ImportPath":"` + runtimeImport + `","GoFiles":["license_startup_default.go"]}`},
		{"invalid_JSON", valid + " \x00"},
		{"empty_graph", ""},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := checkSelected(strings.NewReader(item.graph)); err == nil {
				t.Fatal("unsafe release compiler graph was admitted")
			}
		})
	}
}

const validGoBuildMetadata = `/out/api: go1.27.2
	path	github.com/naseri-coder/TELMIVO/cmd/api
	build	-buildmode=exe
	build	-compiler=gc
	build	CGO_ENABLED=0
	build	GOARCH=amd64
`

func TestLICA9Phase51JRejectBinaryBuildTags(t *testing.T) {
	if err := checkBuildInfo(strings.NewReader(validGoBuildMetadata), "go1.27.2"); err != nil {
		t.Fatalf("clean release binary rejected: %v", err)
	}
	for _, item := range []struct {
		name, info, goVersion string
	}{
		{"browser_tag", validGoBuildMetadata + "\tbuild\t-tags=lic_a9_browser_test\n", "go1.27.2"},
		{"integration_tag", validGoBuildMetadata + "\tbuild\t-tags=integration\n", "go1.27.2"},
		{"wrong_toolchain", validGoBuildMetadata, "go1.27.3"},
		{"prefix_version_attack", strings.Replace(validGoBuildMetadata, "go1.27.2\n", "go1.27.20\n", 1), "go1.27.2"},
		{"CGO_enabled", strings.Replace(validGoBuildMetadata, "CGO_ENABLED=0", "CGO_ENABLED=1", 1), "go1.27.2"},
		{"buildmode_plugin", strings.Replace(validGoBuildMetadata, "-buildmode=exe", "-buildmode=plugin", 1), "go1.27.2"},
		{"incomplete_metadata", "", "go1.27.2"},
	} {
		t.Run(item.name, func(t *testing.T) {
			if err := checkBuildInfo(strings.NewReader(item.info), item.goVersion); err == nil {
				t.Fatal("unauthorized Product binary metadata was admitted")
			}
		})
	}
}
