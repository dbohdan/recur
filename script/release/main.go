package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	checksumFilename = "SHA256SUMS.txt"
	projectName      = "recur"
	distDir          = "dist"
	sshKey           = ".ssh/git"
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
		{"android", "arm64"},
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

	if err := signFile(filepath.Join(releaseDir, checksumFilename)); err != nil {
		fmt.Fprintf(os.Stderr, "Signing failed: %v\n", err)
		os.Exit(1)
	}
}

func build(dir string, target BuildTarget, version string) error {
	fmt.Printf("Building for %s/%s\n", target.os, target.arch)

	ext := ""
	if target.os == "windows" {
		ext = ".exe"
	}

	filename := fmt.Sprintf("%s-v%s-%s-%s%s", projectName, version, target.os, target.arch, ext)
	outputPath := filepath.Join(dir, filename)

	cmd := exec.Command("go", "build", "-trimpath", "-o", outputPath, ".")

	cmd.Env = append(os.Environ(),
		"GOOS="+target.os,
		"GOARCH="+target.arch,
		"CGO_ENABLED=0",
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build command failed: %w\nOutput:\n%s", err, output)
	}

	return generateChecksum(outputPath)
}

func generateChecksum(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksumming: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	hash := hex.EncodeToString(h.Sum(nil))
	checksumLine := fmt.Sprintf("%s  %s\n", hash, filepath.Base(filePath))
	checksumFilePath := filepath.Join(filepath.Dir(filePath), checksumFilename)

	f, err = os.OpenFile(checksumFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open checksum file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(checksumLine); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	return nil
}

func signFile(filePath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	fmt.Printf("Signing %s\n", filePath)

	cmd := exec.Command("ssh-keygen", "-Y", "sign", "-n", "file", "-f", filepath.Join(homeDir, sshKey), filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
