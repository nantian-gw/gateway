package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/nantian-gw/gateway/controlplane/internal/protocompat"
)

const protoPath = "proto/gateway/control/v1/control.proto"

func main() {
	var (
		repoRootFlag = flag.String("repo-root", "", "repository root directory")
		baseRefFlag  = flag.String("base-ref", "", "git ref to compare against")
	)
	flag.Parse()

	repoRoot, err := resolveRepoRoot(*repoRootFlag)
	if err != nil {
		fatalf("resolve repo root: %v", err)
	}

	baseRef := strings.TrimSpace(*baseRefFlag)
	if baseRef == "" {
		baseRef, err = detectBaseRef(repoRoot)
		if err != nil {
			fatalf("detect base ref: %v", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "aeg-proto-compat-*")
	if err != nil {
		fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseProtoRoot := filepath.Join(tempDir, "base-proto")
	baseProtoPath := filepath.Join(baseProtoRoot, "gateway/control/v1/control.proto")
	if err := os.MkdirAll(filepath.Dir(baseProtoPath), 0o755); err != nil {
		fatalf("prepare temp proto tree: %v", err)
	}

	baseProto, err := gitShow(repoRoot, fmt.Sprintf("%s:%s", baseRef, protoPath))
	if err != nil {
		fatalf("load base proto from %s: %v", baseRef, err)
	}
	if err := os.WriteFile(baseProtoPath, baseProto, 0o644); err != nil {
		fatalf("write temp base proto: %v", err)
	}

	protocInclude, err := findProtocInclude()
	if err != nil {
		fatalf("locate protoc include: %v", err)
	}

	currentDescriptorPath := filepath.Join(tempDir, "current.pb")
	baseDescriptorPath := filepath.Join(tempDir, "base.pb")

	if err := compileDescriptor(
		filepath.Join(repoRoot, "proto"),
		filepath.Join(repoRoot, protoPath),
		protocInclude,
		currentDescriptorPath,
	); err != nil {
		fatalf("compile current proto descriptor: %v", err)
	}
	if err := compileDescriptor(
		baseProtoRoot,
		filepath.Join(baseProtoRoot, "gateway/control/v1/control.proto"),
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

	result := protocompat.CompareFiles(baseFile, currentFile)

	fmt.Printf("[proto-compat] base ref: %s\n", baseRef)
	fmt.Printf("[proto-compat] target: %s\n", protoPath)
	for _, warning := range result.Warnings {
		fmt.Printf("[proto-compat] warning: %s: %s\n", warning.Path, warning.Message)
	}
	if !result.OK() {
		for _, finding := range result.Errors {
			fmt.Printf("[proto-compat] error: %s: %s\n", finding.Path, finding.Message)
		}
		os.Exit(1)
	}

	fmt.Println("[proto-compat] compatibility check passed")
}

func resolveRepoRoot(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, statErr := os.Stat(filepath.Join(current, ".git")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("could not find .git directory from current working directory")
		}
		current = parent
	}
}

func detectBaseRef(repoRoot string) (string, error) {
	if tag, err := gitOutput(repoRoot, "describe", "--tags", "--abbrev=0", "--match", "v*"); err == nil {
		return strings.TrimSpace(tag), nil
	}

	headParent, err := gitOutput(repoRoot, "rev-parse", "--verify", "HEAD^")
	if err != nil {
		return "", errors.New("no release tag and no parent commit available for proto compatibility check")
	}
	return strings.TrimSpace(headParent), nil
}

func gitShow(repoRoot, object string) ([]byte, error) {
	cmd := exec.Command("git", "-C", repoRoot, "show", object)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func findProtocInclude() (string, error) {
	if include := strings.TrimSpace(os.Getenv("PROTOC_INCLUDE")); include != "" {
		if wellKnownTypeExists(include) {
			return include, nil
		}
		return "", fmt.Errorf("PROTOC_INCLUDE does not contain google/protobuf well-known types: %s", include)
	}

	protocPath, err := exec.LookPath("protoc")
	if err != nil {
		return "", errors.New("protoc not found in PATH")
	}
	protocRoot := filepath.Dir(filepath.Dir(protocPath))

	candidates := []string{
		filepath.Join(protocRoot, "include"),
		"/usr/local/include",
		"/usr/include",
		"/opt/homebrew/include",
		"/opt/local/include",
	}
	for _, candidate := range candidates {
		if wellKnownTypeExists(candidate) {
			return candidate, nil
		}
	}

	searchRoots := []string{"/usr/local", "/usr", "/opt/homebrew", "/opt/local"}
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
	_, err := os.Stat(filepath.Join(root, "google/protobuf/duration.proto"))
	return err == nil
}

func findWellKnownType(root string) (string, error) {
	var found string
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
	cmd := exec.Command(
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
	data, err := os.ReadFile(path)
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
