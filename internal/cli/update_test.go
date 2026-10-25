package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeGo instala un "go" falso que registra args/entorno en log y deja en
// $GOBIN un "tg" que responde a `version --json` con newVersion.
func fakeGo(t *testing.T, exitCode int, newVersion string) (goPath, logPath, binDir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake go usa sh")
	}
	dir := t.TempDir()
	binDir = filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(dir, "log")
	goPath = filepath.Join(dir, "go")
	script := `#!/bin/sh
echo "args=$*" > "` + logPath + `"
echo "GOBIN=$GOBIN" >> "` + logPath + `"
echo "CGO_ENABLED=$CGO_ENABLED" >> "` + logPath + `"
if [ ` + strconv.Itoa(exitCode) + ` -ne 0 ]; then echo "boom" >&2; exit ` + strconv.Itoa(exitCode) + `; fi
printf '#!/bin/sh\necho '"'"'{"version":"` + newVersion + `","commit":"x","bot_api":"10.3"}'"'"'\n' > "$GOBIN/tg"
chmod +x "$GOBIN/tg"
`
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return goPath, logPath, binDir
}

func withUpdateStubs(t *testing.T, goPath, exe string) {
	t.Helper()
	oldLook, oldExe := lookPath, executablePath
	t.Cleanup(func() { lookPath, executablePath = oldLook, oldExe })
	lookPath = func(string) (string, error) {
		if goPath == "" {
			return "", exec.ErrNotFound
		}
		return goPath, nil
	}
	executablePath = func() (string, error) { return exe, nil }
}

// captureStd ejecuta f capturando stdout y stderr.
func captureStd(t *testing.T, f func() int) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	os.Stdout, os.Stderr = wo, we
	outC := make(chan string)
	errC := make(chan string)
	go func() { b, _ := io.ReadAll(ro); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(re); errC <- string(b) }()
	code = f()
	wo.Close()
	we.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	return code, <-outC, <-errC
}

func TestUpdate_Success_ReplacesRunningBinaryDir(t *testing.T) {
	goPath, logPath, binDir := fakeGo(t, 0, "v0.2.0")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))

	code, out, errOut := captureStd(t, func() int {
		return Run([]string{"update"}, VersionInfo{Version: "v0.1.0", Commit: "abc"})
	})
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "v0.1.0 → v0.2.0") {
		t.Fatalf("stdout=%q", out)
	}
	log, _ := os.ReadFile(logPath)
	for _, want := range []string{
		"args=install -ldflags=-s -w github.com/xhelodev/tg/cmd/tg@latest",
		"GOBIN=" + binDir,
		"CGO_ENABLED=0",
	} {
		if !strings.Contains(string(log), want) {
			t.Fatalf("falta %q en log:\n%s", want, log)
		}
	}
}

func TestUpdate_JSONAndRef(t *testing.T) {
	goPath, logPath, binDir := fakeGo(t, 0, "v0.3.0")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))

	code, out, errOut := captureStd(t, func() int {
		return Run([]string{"update", "--json", "--ref", "v0.3.0"}, VersionInfo{Version: "v0.1.0"})
	})
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout no es JSON: %q", out)
	}
	if got["previous"] != "v0.1.0" || got["current"] != "v0.3.0" || got["ok"] != true {
		t.Fatalf("json=%v", got)
	}
	log, _ := os.ReadFile(logPath)
	if !strings.Contains(string(log), "tg/cmd/tg@v0.3.0") {
		t.Fatalf("ref no usado: %s", log)
	}
}

func TestUpdate_AlreadyCurrent(t *testing.T) {
	goPath, _, binDir := fakeGo(t, 0, "v0.1.0")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))
	code, out, _ := captureStd(t, func() int {
		return Run([]string{"update"}, VersionInfo{Version: "v0.1.0"})
	})
	if code != ExitOK || !strings.Contains(out, "ya estaba al día") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUpdate_RenamedBinaryUsesDefaultGOBIN(t *testing.T) {
	goPath, logPath, binDir := fakeGo(t, 0, "v0.2.0")
	t.Setenv("GOBIN", binDir) // el default de go del usuario
	withUpdateStubs(t, goPath, filepath.Join(t.TempDir(), "cli.test"))
	code, out, errOut := captureStd(t, func() int {
		return Run([]string{"update"}, VersionInfo{})
	})
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "tg actualizado") {
		t.Fatalf("out=%q", out)
	}
	log, _ := os.ReadFile(logPath)
	if !strings.Contains(string(log), "GOBIN="+binDir) {
		t.Fatalf("no debe forzar GOBIN si el binario no se llama tg: %s", log)
	}
}

func TestUpdate_DryRunDoesNotExecute(t *testing.T) {
	goPath, logPath, binDir := fakeGo(t, 0, "v0.2.0")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"update", "--dry-run"}, VersionInfo{})
	})
	if code != ExitOK {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errOut, "install -ldflags=-s -w github.com/xhelodev/tg/cmd/tg@latest") {
		t.Fatalf("stderr=%q", errOut)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run ejecutó go")
	}
}

func TestUpdate_GoInstallFails(t *testing.T) {
	goPath, _, binDir := fakeGo(t, 1, "")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"update"}, VersionInfo{})
	})
	if code != ExitNetwork {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(errOut, "boom") || !strings.Contains(errOut, "go install falló") {
		t.Fatalf("stderr=%q", errOut)
	}
}

func TestUpdate_NoGo(t *testing.T) {
	withUpdateStubs(t, "", "/x/tg")
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"update"}, VersionInfo{})
	})
	if code != ExitConfig || !strings.Contains(errOut, "'go'") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestUpdate_BadUsage(t *testing.T) {
	withUpdateStubs(t, "/nonexistent/go", "/x/tg")
	for _, args := range [][]string{
		{"update", "--ref", "-x"},
		{"update", "--ref", "v1 && rm"},
		{"update", "--ref", ""},
		{"update", "extra"},
		{"update", "--nope"},
	} {
		code, _, _ := captureStd(t, func() int { return Run(args, VersionInfo{}) })
		if code != ExitUsage {
			t.Fatalf("%v: code=%d", args, code)
		}
	}
}

func TestUpdate_Interrupted(t *testing.T) {
	goPath, _, binDir := fakeGo(t, 0, "v0.2.0")
	withUpdateStubs(t, goPath, filepath.Join(binDir, "tg"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, _, _ := captureStd(t, func() int { return cmdUpdate(ctx, nil, VersionInfo{}) })
	if code != ExitInterrupted {
		t.Fatalf("code=%d", code)
	}
}

func TestResolveVersion_KeepsLdflags(t *testing.T) {
	got := resolveVersion(VersionInfo{Version: "v9.9.9", Commit: "deadbeef"})
	if got.Version != "v9.9.9" || got.Commit != "deadbeef" {
		t.Fatalf("%+v", got)
	}
	// Sin ldflags en un test binary: Main.Version es "(devel)" o vacío → "dev".
	if got := resolveVersion(VersionInfo{}); got.Version == "" || got.Commit == "" {
		t.Fatalf("%+v", got)
	}
}
