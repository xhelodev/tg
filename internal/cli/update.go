package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// modulePath es el paquete que reinstala `tg update`.
const modulePath = "github.com/xhelodev/tg/cmd/tg"

// Inyectables en tests.
var (
	lookPath       = exec.LookPath
	executablePath = os.Executable
)

// refPattern acepta versiones (v1.2.3), "latest", ramas y hashes; nada que
// empiece por "-" ni espacios.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+-]*$`)

// resolveVersion completa la versión con la info de build de Go cuando el
// binario no se compiló con -ldflags (caso `go install …@latest`).
func resolveVersion(ver VersionInfo) VersionInfo {
	if ver.Version == "" {
		ver.Version = "dev"
	}
	if ver.Commit == "" {
		ver.Commit = "unknown"
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ver
	}
	if ver.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		ver.Version = bi.Main.Version
	}
	if ver.Commit == "unknown" {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				ver.Commit = s.Value
				if len(ver.Commit) > 12 {
					ver.Commit = ver.Commit[:12]
				}
			}
		}
	}
	return ver
}

// cmdUpdate reinstala tg con `go install` en el mismo directorio del binario
// en ejecución (GOBIN), para reemplazarlo aunque no viva en GOPATH/bin.
func cmdUpdate(ctx context.Context, args []string, ver VersionInfo) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var ref string
	var jsonOut, quiet, dryRun bool
	fs.StringVar(&ref, "ref", "latest", "versión, rama o commit (default latest)")
	fs.BoolVar(&jsonOut, "json", false, "")
	fs.BoolVar(&quiet, "quiet", false, "")
	fs.BoolVar(&quiet, "q", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	if err := fs.Parse(args); err != nil {
		PrintErr("flags: %v", err)
		return ExitUsage
	}
	if fs.NArg() > 0 {
		PrintErr("update no acepta argumentos posicionales (use --ref)")
		return ExitUsage
	}
	if !refPattern.MatchString(ref) {
		PrintErr("--ref inválido: %q", ref)
		return ExitUsage
	}

	goBin, err := lookPath("go")
	if err != nil {
		PrintErr("no se encontró 'go' en PATH; tg update requiere Go ≥ 1.22 (o reinstale con install.sh)")
		return ExitConfig
	}

	before := resolveVersion(ver)
	gobin := installDir()
	cmdArgs := []string{"install", "-ldflags=-s -w", modulePath + "@" + ref}
	env := append(os.Environ(), "CGO_ENABLED=0")
	if gobin != "" {
		env = append(env, "GOBIN="+gobin)
	}

	if dryRun {
		dest := gobin
		if dest == "" {
			dest = "(GOBIN / GOPATH/bin)"
		}
		fmt.Fprintf(os.Stderr, "+ CGO_ENABLED=0 GOBIN=%s %s %s\n", dest, goBin, strings.Join(cmdArgs, " "))
		return ExitOK
	}

	if !quiet && !jsonOut {
		PrintErr("actualizando tg %s → %s@%s", before.Version, modulePath, ref)
	}
	cmd := exec.CommandContext(ctx, goBin, cmdArgs...)
	cmd.Env = env
	// stdout limpio para --json: todo lo de go va a stderr.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ExitInterrupted
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			hint := ""
			if ref == "latest" {
				hint = " o pruebe --ref main"
			}
			PrintErr("go install falló (exit %d); revise red/GOPROXY%s", ee.ExitCode(), hint)
		} else {
			PrintErr("go install: %v", err)
		}
		return ExitNetwork
	}

	after := installedVersion(ctx, gobin)
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok": true, "previous": before.Version, "current": after,
		})
		return ExitOK
	}
	if !quiet {
		if after == "" {
			fmt.Println("tg actualizado")
		} else if after == before.Version {
			fmt.Printf("tg ya estaba al día (%s)\n", after)
		} else {
			fmt.Printf("tg actualizado: %s → %s\n", before.Version, after)
		}
	}
	return ExitOK
}

// installDir devuelve el directorio del binario en ejecución si se llama
// "tg"; si no (renombrado, go run, tests), "" para usar el default de go.
func installDir() string {
	exe, err := executablePath()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if strings.TrimSuffix(filepath.Base(exe), ".exe") != "tg" {
		return ""
	}
	return filepath.Dir(exe)
}

// installedVersion ejecuta el binario recién instalado para leer su versión.
// Best effort: "" si no se puede determinar.
func installedVersion(ctx context.Context, gobin string) string {
	if gobin == "" {
		return ""
	}
	name := "tg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out, err := exec.CommandContext(ctx, filepath.Join(gobin, name), "version", "--json").Output()
	if err != nil {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(out, &v) != nil {
		return ""
	}
	return v.Version
}
