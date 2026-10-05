package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const installStubBinaryContents = "fake-binary\n"

// installScriptAsset mirrors the OS/architecture mapping in install.sh for the
// platform the test runs on.
func installScriptAsset(t *testing.T, version string) string {
	t.Helper()

	var osLabel string
	switch runtime.GOOS {
	case "darwin":
		osLabel = "macOS"
	case "linux":
		osLabel = "linux"
	default:
		t.Skipf("install.sh does not support GOOS %q", runtime.GOOS)
	}

	var archLabel string
	switch runtime.GOARCH {
	case "amd64":
		archLabel = "amd64"
	case "arm64":
		archLabel = "arm64"
	default:
		t.Skipf("install.sh does not support GOARCH %q", runtime.GOARCH)
	}

	return fmt.Sprintf("asc_%s_%s_%s", version, osLabel, archLabel)
}

// runInstallScript executes install.sh with a stubbed curl so no network
// access happens. checksumsMode controls what the stub serves for
// checksums.txt: valid, wrong, unlisted, or missing.
func runInstallScript(t *testing.T, checksumsMode string, extraEnv ...string) (output, installedBinary string, err error) {
	t.Helper()

	repoRoot, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("getwd: %v", wdErr)
	}

	version := "0.0.1"
	asset := installScriptAsset(t, version)
	sum := sha256.Sum256([]byte(installStubBinaryContents))
	checksum := hex.EncodeToString(sum[:])

	workDir := t.TempDir()
	stubDir := filepath.Join(workDir, "stub-bin")
	installDir := filepath.Join(workDir, "install-bin")
	if mkErr := os.MkdirAll(stubDir, 0o755); mkErr != nil {
		t.Fatalf("mkdir stub dir: %v", mkErr)
	}

	curlStub := `#!/usr/bin/env bash
set -euo pipefail
out=""
url=""
prev=""
for arg in "$@"; do
  if [ "${prev}" = "-o" ]; then
    out="${arg}"
  fi
  case "${arg}" in
    http://*|https://*) url="${arg}" ;;
  esac
  prev="${arg}"
done

retry_target=""
case "${url}" in
  *_checksums.txt) retry_target="checksums" ;;
  */releases/download/*) retry_target="binary" ;;
esac
if [ -n "${retry_target}" ] && [ "${STUB_FAIL_ONCE:-}" = "${retry_target}" ]; then
  attempt_file="${STUB_STATE_DIR}/${retry_target}-attempted"
  if [ ! -e "${attempt_file}" ]; then
    touch "${attempt_file}"
    exit 22
  fi
fi

case "${url}" in
  */releases/latest)
    printf '%s' "https://github.com/rorkai/App-Store-Connect-CLI/releases/tag/${STUB_VERSION}"
    ;;
  *_checksums.txt)
    case "${STUB_CHECKSUMS_MODE}" in
      missing) exit 22 ;;
      valid) printf '%s  %s\n' "${STUB_SHA256}" "${STUB_ASSET}" > "${out}" ;;
      unlisted) printf '%s  %s\n' "${STUB_SHA256}" "${STUB_ASSET//./x}" > "${out}" ;;
      wrong) printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" "${STUB_ASSET}" > "${out}" ;;
      *) echo "unknown STUB_CHECKSUMS_MODE" >&2; exit 1 ;;
    esac
    ;;
  *)
    printf 'fake-binary\n' > "${out}"
    ;;
esac
`
	if writeErr := os.WriteFile(filepath.Join(stubDir, "curl"), []byte(curlStub), 0o755); writeErr != nil {
		t.Fatalf("write curl stub: %v", writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(stubDir, "sleep"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); writeErr != nil {
		t.Fatalf("write sleep stub: %v", writeErr)
	}

	cmd := exec.Command("bash", filepath.Join(repoRoot, "install.sh"))
	cmd.Dir = workDir
	cmd.Env = append(
		os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"INSTALL_DIR="+installDir,
		"STUB_VERSION="+version,
		"STUB_ASSET="+asset,
		"STUB_SHA256="+checksum,
		"STUB_CHECKSUMS_MODE="+checksumsMode,
		"STUB_STATE_DIR="+workDir,
	)
	cmd.Env = append(cmd.Env, extraEnv...)

	combined, runErr := cmd.CombinedOutput()
	return string(combined), filepath.Join(installDir, "asc"), runErr
}

func TestInstallScriptRetriesTransientBinaryDownloadFailure(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "valid", "STUB_FAIL_ONCE=binary")
	if err != nil {
		t.Fatalf("expected transient binary download failure to recover: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Download failed; retrying (2/3)") {
		t.Fatalf("expected retry diagnostic, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); statErr != nil {
		t.Fatalf("expected binary installed at %s: %v\n%s", installedBinary, statErr, output)
	}
}

func TestInstallScriptRetriesTransientChecksumDownloadFailure(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "valid", "STUB_FAIL_ONCE=checksums")
	if err != nil {
		t.Fatalf("expected transient checksum download failure to recover: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Download failed; retrying (2/3)") {
		t.Fatalf("expected retry diagnostic, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); statErr != nil {
		t.Fatalf("expected binary installed at %s: %v\n%s", installedBinary, statErr, output)
	}
}

func TestInstallScriptSyntaxIsValid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh targets unix shells")
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cmd := exec.Command("bash", "-n", filepath.Join(repoRoot, "install.sh"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n install.sh failed: %v\n%s", err, output)
	}
}

func TestInstallScriptVerifiesChecksumAndInstalls(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "valid")
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Checksum verified.") {
		t.Fatalf("expected checksum verification confirmation, got:\n%s", output)
	}
	contents, readErr := os.ReadFile(installedBinary)
	if readErr != nil {
		t.Fatalf("expected installed binary at %s: %v\n%s", installedBinary, readErr, output)
	}
	if string(contents) != installStubBinaryContents {
		t.Fatalf("installed binary contents mismatch: %q", contents)
	}
}

func TestInstallScriptFailsClosedWhenChecksumsMissing(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "missing")
	if err == nil {
		t.Fatalf("expected install.sh to fail without checksums.txt, got success:\n%s", output)
	}
	if !strings.Contains(output, "Refusing to install without SHA-256 checksum verification.") {
		t.Fatalf("expected fail-closed error message, got:\n%s", output)
	}
	if !strings.Contains(output, "ASC_INSTALL_INSECURE=1") {
		t.Fatalf("expected override hint in error output, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); !os.IsNotExist(statErr) {
		t.Fatalf("expected no binary installed, stat err=%v\n%s", statErr, output)
	}
}

func TestInstallScriptFailsClosedWhenAssetUnlisted(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "unlisted")
	if err == nil {
		t.Fatalf("expected install.sh to fail when asset is missing from checksums.txt, got success:\n%s", output)
	}
	if !strings.Contains(output, "Refusing to install without SHA-256 checksum verification.") {
		t.Fatalf("expected fail-closed error message, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); !os.IsNotExist(statErr) {
		t.Fatalf("expected no binary installed, stat err=%v\n%s", statErr, output)
	}
}

func TestInstallScriptInsecureOverrideAllowsMissingChecksums(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "missing", "ASC_INSTALL_INSECURE=1")
	if err != nil {
		t.Fatalf("expected ASC_INSTALL_INSECURE=1 to allow install, got: %v\n%s", err, output)
	}
	if !strings.Contains(output, "installing WITHOUT checksum verification") {
		t.Fatalf("expected loud insecure-install warning, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); statErr != nil {
		t.Fatalf("expected binary installed at %s: %v\n%s", installedBinary, statErr, output)
	}
}

func TestInstallScriptChecksumMismatchFailsEvenWithInsecureOverride(t *testing.T) {
	output, installedBinary, err := runInstallScript(t, "wrong", "ASC_INSTALL_INSECURE=1")
	if err == nil {
		t.Fatalf("expected install.sh to fail on checksum mismatch, got success:\n%s", output)
	}
	if !strings.Contains(output, "Checksum verification failed") {
		t.Fatalf("expected checksum mismatch error, got:\n%s", output)
	}
	if _, statErr := os.Stat(installedBinary); !os.IsNotExist(statErr) {
		t.Fatalf("expected no binary installed, stat err=%v\n%s", statErr, output)
	}
}
