package validation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var containerEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var containerImageDigest = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// ContainerExecutor runs untrusted validation commands in a locked-down
// Docker container. The workspace is the only host path mounted into it.
type ContainerExecutor struct {
	Image string
}

func NewContainerExecutor(image string) *ContainerExecutor {
	return &ContainerExecutor{Image: strings.TrimSpace(image)}
}

func (e *ContainerExecutor) Execute(ctx context.Context, dir string, env map[string]string, command ...string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("empty command")
	}
	if !pinnedContainerImage(e.Image) {
		return nil, errors.New("validation sandbox image must be pinned by sha256 digest")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve validation workspace: %w", err)
	}
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 {
		uid, gid = 65534, 65534
		if err := filepath.Walk(absDir, func(path string, _ os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			return os.Chown(path, uid, gid)
		}); err != nil {
			return nil, fmt.Errorf("isolate validation workspace ownership: %w", err)
		}
	}
	args := []string{
		"run", "--rm", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "256", "--memory", "1g", "--cpus", "2",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid), "--workdir", "/workspace",
		"--mount", "type=bind,src=" + absDir + ",dst=/workspace,rw",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=268435456",
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		if !containerEnvName.MatchString(key) {
			return nil, fmt.Errorf("invalid validation environment key %q", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+env[key])
	}
	args = append(args, e.Image)
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/nonexistent"}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

func pinnedContainerImage(image string) bool {
	if strings.HasPrefix(image, "sha256:") {
		return containerImageDigest.MatchString(strings.TrimPrefix(image, "sha256:"))
	}
	parts := strings.Split(image, "@sha256:")
	return len(parts) == 2 && parts[0] != "" && containerImageDigest.MatchString(parts[1])
}

var _ Executor = (*ContainerExecutor)(nil)
