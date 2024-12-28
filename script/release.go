package main

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	checksumFilename = "SHA512SUMS.txt"
	projectName      = "recur"
	distDir          = "dist"
)

type BuildTarget struct {
	os   string
	arch string
}

func main() {
	version := os.Getenv("VERSION")
	if version == "" {
		fmt.Fprintln(os.Stderr, "'VERSION' environment variable must be set")
		os.Exit(1)
	}

	releaseDir := filepath.Join(distDir, version)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create release directory: %v\n", err)
		os.Exit(1)
	}

	targets := []BuildTarget{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"freebsd", "amd64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"linux", "riscv64"},
		{"netbsd", "amd64"},
		{"openbsd", "amd64"},
		{"windows", "386"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}

	for _, target := range targets {
		if err := build(releaseDir, target, version); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed for %s/%s: %v\n", target.os, target.arch, err)
			os.Exit(1)
		}
	}
}

func build(dir string, target BuildTarget, version string) error {
	fmt.Printf("Building for %s/%s\n", target.os, target.arch)

	ext := ""
	if target.os == "windows" {
		ext = ".exe"
	}

	// Map `GOARCH` and `GOOS` to user-facing names.
	arch := target.arch
	system := target.os

	if arch == "386" {
		arch = "x86"
	}
	if system == "darwin" {
		system = "macos"
	}
	if (system == "linux" || system == "macos") && arch == "amd64" {
		arch = "x86_64"
	}
	if system == "linux" && arch == "arm64" {
		arch = "aarch64"
	}

	filename := fmt.Sprintf("%s-v%s-%s-%s%s", projectName, version, system, arch, ext)
	outputPath := filepath.Join(dir, filename)

	cmd := exec.Command("go", "build", "-trimpath", "-o", outputPath, ".")
	cmd.Env = append(os.Environ(),
		"GOOS="+target.os,
		"GOARCH="+target.arch,
		"CGO_ENABLED=0",
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Build command failed: %v\nOutput:\n%s", err, output)
	}

	return generateChecksum(outputPath, version)
}

func generateChecksum(filePath, version string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("Failed to open file for checksumming: %v", err)
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("Failed to calculate hash: %v", err)
	}

	hash := hex.EncodeToString(h.Sum(nil))

	checksumLine := fmt.Sprintf("%s  %s\n", hash, filepath.Base(filePath))

	checksumFilePath := filepath.Join(filepath.Dir(filePath), checksumFilename)
	f, err = os.OpenFile(checksumFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open checksum file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(checksumLine); err != nil {
		return fmt.Errorf("Failed to write checksum: %v", err)
	}

	return nil
}
