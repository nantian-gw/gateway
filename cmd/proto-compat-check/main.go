package main

import (
	jsoniter "github.com/json-iterator/go"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/nantian-gw/gateway/internal/compat"
)

const (
	protoModule  = "github.com/nantian-gw/proto"
	protoRelPath = "gateway/control/v1/control.proto"
)

func main() {
	var (
		baseVersionFlag = flag.String("base-version", "", "proto module version to compare against (required)")
		repoRootFlag    = flag.String("repo-root", "", "gateway repository root directory")
	)
	flag.Parse()

	baseVersion := strings.TrimSpace(*baseVersionFlag)
	if baseVersion == "" {
		fatalf("--base-version is required (e.g. v0.1.0 or a pseudo-version)")
	}

	repoRoot, err := resolveRepoRoot(*repoRootFlag)
	if err != nil {
		fatalf("resolve repo root: %v", err)
	}

	currentProtoDir, err := moduleDir(repoRoot, protoModule+"@latest")
	if err != nil {
		fatalf("resolve current proto module: %v", err)
	}

	baseProtoDir, err := moduleDir(repoRoot, protoModule+"@"+baseVersion)
	if err != nil {
		fatalf("resolve base proto module (%s): %v", baseVersion, err)
	}

	protocInclude, err := findProtocInclude()
	if err != nil {
		fatalf("locate protoc include: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "proto-compat-*")
	if err != nil {
		fatalf("create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	currentDescriptorPath := filepath.Join(tempDir, "current.pb")
	baseDescriptorPath := filepath.Join(tempDir, "base.pb")

	if err := compileDescriptor(
		currentProtoDir,
		filepath.Join(currentProtoDir, protoRelPath),
		protocInclude,
		currentDescriptorPath,
	); err != nil {
		fatalf("compile current proto descriptor: %v", err)
	}
	if err := compileDescriptor(
		baseProtoDir,
		filepath.Join(baseProtoDir, protoRelPath),
		protocInclude,
		baseDescriptorPath,
	); err != nil {
		fatalf("compile base proto descriptor: %v", err)
	}

	currentSet, err := loadDescriptorSet(currentDescriptorPath)
	if err != nil {
		fatalf("load current descriptor set: %v", err)
	}
	baseSet, err := loadDescriptorSet(baseDescriptorPath)
	if err != nil {
		fatalf("load base descriptor set: %v", err)
	}

	currentFile, err := findDescriptorFile(currentSet, "gateway/control/v1/control.proto")
	if err != nil {
		fatalf("find current target descriptor: %v", err)
	}
	baseFile, err := findDescriptorFile(baseSet, "gateway/control/v1/control.proto")
	if err != nil {
		fatalf("find base target descriptor: %v", err)
	}

	result := compat.CompareFiles(baseFile, currentFile)
	if !result.OK() {
		fmt.Println("Backward-incompatible changes detected:")
		for _, f := range result.Errors {
			fmt.Printf("  ERROR: %s: %s\n", f.Path, f.Message)
		}
	}
	for _, f := range result.Warnings {
		fmt.Printf("  WARNING: %s: %s\n", f.Path, f.Message)
	}

	if !result.OK() {
		os.Exit(1)
	}
	fmt.Println("Proto is backward compatible.")
}

type moduleInfo struct {
	Dir string `json:"Dir"`
}

func moduleDir(repoRoot, moduleQuery string) (string, error) {
	download := exec.Command("go", "mod", "download", "-json", moduleQuery) //nolint:gosec
	download.Dir = repoRoot
	download.Stderr = os.Stderr
	downloadOutput, err := download.Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s: %w", moduleQuery, err)
	}

	var info moduleInfo
	if err := jsoniter.Unmarshal(downloadOutput, &info); err != nil {
		return "", fmt.Errorf("parse go mod download output: %w", err)
	}
	if info.Dir == "" {
		return "", fmt.Errorf("module %s: Dir is empty", moduleQuery)
	}
	return info.Dir, nil
}

func resolveRepoRoot(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find git root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func findProtocInclude() (string, error) {
	if env := os.Getenv("PROTOC_INCLUDE"); env != "" {
		return env, nil
	}

	searchRoots := []string{
		"/usr/local/include",
		"/usr/include",
		filepath.Join(os.Getenv("HOME"), ".local/include"),
	}

	for _, root := range searchRoots {
		if wellKnownTypeExists(root) {
			return root, nil
		}
	}

	for _, root := range searchRoots {
		candidate, err := findWellKnownType(root)
		if err != nil {
			continue
		}
		return candidate, nil
	}

	return "", errors.New("unable to locate google/protobuf well-known types; install protobuf headers or set PROTOC_INCLUDE")
}

func wellKnownTypeExists(root string) bool {
	_, err := os.Stat(filepath.Join(root, "google/protobuf/duration.proto")) //nolint:gosec
	return err == nil
}

func findWellKnownType(root string) (string, error) {
	var found string
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error { //nolint:gosec
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "google/protobuf/duration.proto") {
			found = filepath.Dir(filepath.Dir(filepath.Dir(path)))
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if found == "" {
		return "", errors.New("not found")
	}
	return found, nil
}

func compileDescriptor(protoRoot, protoFile, protocInclude, outputPath string) error {
	cmd := exec.Command( //nolint:gosec
		"protoc",
		"-I", protoRoot,
		"-I", protocInclude,
		"--include_imports",
		"--descriptor_set_out", outputPath,
		protoFile,
	)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func loadDescriptorSet(path string) (*descriptorpb.FileDescriptorSet, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, err
	}
	return &set, nil
}

func findDescriptorFile(set *descriptorpb.FileDescriptorSet, name string) (*descriptorpb.FileDescriptorProto, error) {
	for _, file := range set.GetFile() {
		if file.GetName() == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("descriptor file %q not found", name)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[proto-compat] "+format+"\n", args...)
	os.Exit(1)
}
