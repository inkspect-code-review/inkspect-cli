//go:build ignore

// Build release archives without Git, Docker, C toolchains or external packages.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	if e := build(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func build() error {
	version := "0.1.2"
	if len(os.Args) > 1 {
		version = os.Args[1]
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		return fmt.Errorf("invalid version")
	}
	if e := os.MkdirAll("dist", 0755); e != nil {
		return e
	}
	sums := []string{}
	for _, target := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		osName, arch, _ := strings.Cut(target, "/")
		name := "inkspect"
		ext := ".tar.gz"
		if osName == "windows" {
			name += ".exe"
			ext = ".zip"
		}
		temp, e := os.MkdirTemp("", "inkspect-release-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(temp)
		binary := filepath.Join(temp, name)
		cmd := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-ldflags", "-s -w -X github.com/inkspect-code-review/inkspect-cli/internal/cli.Version="+version, "-o", binary, "./cmd/inkspect")
		cmd.Env = append(os.Environ(), "GOOS="+osName, "GOARCH="+arch, "CGO_ENABLED=0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			return e
		}
		data, e := os.ReadFile(binary)
		if e != nil {
			return e
		}
		asset := "inkspect_" + version + "_" + osName + "_" + arch + ext
		f, e := os.Create(filepath.Join("dist", asset))
		if e != nil {
			return e
		}
		if osName == "windows" {
			z := zip.NewWriter(f)
			w, e := z.Create(name)
			if e != nil {
				return e
			}
			if _, e = w.Write(data); e != nil {
				return e
			}
			if e = z.Close(); e != nil {
				return e
			}
		} else {
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), ModTime: time.Unix(0, 0)}); e != nil {
				return e
			}
			if _, e = tw.Write(data); e != nil {
				return e
			}
			if e = tw.Close(); e != nil {
				return e
			}
			if e = gz.Close(); e != nil {
				return e
			}
		}
		if e = f.Close(); e != nil {
			return e
		}
		archive, e := os.ReadFile(filepath.Join("dist", asset))
		if e != nil {
			return e
		}
		sums = append(sums, fmt.Sprintf("%x  %s", sha256.Sum256(archive), asset))
		fmt.Println(asset)
	}
	return os.WriteFile("dist/SHA256SUMS", []byte(strings.Join(sums, "\n")+"\n"), 0644)
}
