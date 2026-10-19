// Package container wraps the Apple `container` CLI.
package container

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// Client runs the `container` binary and parses its output.
type Client struct {
	bin string
}

// New locates the container binary on PATH.
func New() (*Client, error) {
	bin, err := exec.LookPath("container")
	if err != nil {
		return nil, fmt.Errorf("container CLI not found on PATH: %w", err)
	}
	return &Client{bin: bin}, nil
}

// Binary returns the resolved path of the container CLI.
func (c *Client) Binary() string { return c.bin }

func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	return runJSON[[]Container](c, ctx, "ls", "-a", "--format", "json")
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	return runJSON[[]Image](c, ctx, "image", "ls", "--format", "json")
}

func (c *Client) ListVolumes(ctx context.Context) ([]Volume, error) {
	return runJSON[[]Volume](c, ctx, "volume", "ls", "--format", "json")
}

func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	return runJSON[[]Network](c, ctx, "network", "ls", "--format", "json")
}

func (c *Client) Stats(ctx context.Context) ([]Stats, error) {
	return runJSON[[]Stats](c, ctx, "stats", "--no-stream", "--format", "json")
}

func (c *Client) DiskUsage(ctx context.Context) (DiskUsage, error) {
	return runJSON[DiskUsage](c, ctx, "system", "df", "--format", "json")
}

// SystemStatus parses the FIELD/VALUE table from `container system status`.
func (c *Client) SystemStatus(ctx context.Context) (SystemStatus, error) {
	out, err := c.run(ctx, "system", "status")
	if err != nil {
		return SystemStatus{}, err
	}
	return parseSystemStatus(out), nil
}

func (c *Client) InspectContainer(ctx context.Context, id string) (string, error) {
	return c.runString(ctx, "inspect", id)
}

func (c *Client) InspectImage(ctx context.Context, name string) (string, error) {
	return c.runString(ctx, "image", "inspect", name)
}

func (c *Client) InspectVolume(ctx context.Context, name string) (string, error) {
	return c.runString(ctx, "volume", "inspect", name)
}

func (c *Client) InspectNetwork(ctx context.Context, name string) (string, error) {
	return c.runString(ctx, "network", "inspect", name)
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	_, err := c.run(ctx, "start", id)
	return err
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	_, err := c.run(ctx, "stop", id)
	return err
}

func (c *Client) KillContainer(ctx context.Context, id string) error {
	_, err := c.run(ctx, "kill", id)
	return err
}

func (c *Client) DeleteContainer(ctx context.Context, id string, force bool) error {
	args := []string{"delete"}
	if force {
		args = append(args, "--force")
	}
	_, err := c.run(ctx, append(args, id)...)
	return err
}

func (c *Client) PruneContainers(ctx context.Context) error {
	_, err := c.run(ctx, "prune")
	return err
}

func (c *Client) DeleteImage(ctx context.Context, name string) error {
	_, err := c.run(ctx, "image", "delete", name)
	return err
}

func (c *Client) PruneImages(ctx context.Context, all bool) error {
	args := []string{"image", "prune"}
	if all {
		args = append(args, "--all")
	}
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) DeleteVolume(ctx context.Context, name string) error {
	_, err := c.run(ctx, "volume", "delete", name)
	return err
}

func (c *Client) PruneVolumes(ctx context.Context) error {
	_, err := c.run(ctx, "volume", "prune")
	return err
}

func (c *Client) DeleteNetwork(ctx context.Context, name string) error {
	_, err := c.run(ctx, "network", "delete", name)
	return err
}

func (c *Client) PruneNetworks(ctx context.Context) error {
	_, err := c.run(ctx, "network", "prune")
	return err
}

// ExecCommand builds an interactive `container exec -it` command for the caller to run.
func (c *Client) ExecCommand(id string, argv ...string) *exec.Cmd {
	args := append([]string{"exec", "-it", id}, argv...)
	return exec.Command(c.bin, args...) //nolint:gosec // argv is user-chosen by design
}

// Logs streams log lines for a container until ctx is cancelled or the process exits.
// The returned channel is closed when streaming stops.
func (c *Client) Logs(ctx context.Context, id string, tail int) (<-chan string, error) {
	args := []string{"logs", "-f"}
	if tail > 0 {
		args = append(args, "-n", strconv.Itoa(tail))
	}
	cmd := exec.CommandContext(ctx, c.bin, append(args, id)...) //nolint:gosec // id comes from container ls
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("logs pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start logs: %w", err)
	}
	lines := make(chan string, 1024)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				_ = cmd.Wait()
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			lines <- "log stream error: " + err.Error()
		}
		_ = cmd.Wait()
	}()
	return lines, nil
}

func runJSON[T any](c *Client, ctx context.Context, args ...string) (T, error) {
	var v T
	out, err := c.run(ctx, args...)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return v, fmt.Errorf("parse %s output: %w", strings.Join(args, " "), err)
	}
	return v, nil
}

func (c *Client) runString(ctx context.Context, args ...string) (string, error) {
	out, err := c.run(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.bin, args...) //nolint:gosec // args are fixed verbs plus resource IDs
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := lastLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("container %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, line := range slices.Backward(lines) {
		if l := strings.TrimSpace(line); l != "" {
			return strings.TrimPrefix(l, "Error: ")
		}
	}
	return ""
}

func parseSystemStatus(out []byte) SystemStatus {
	var st SystemStatus
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "status":
			st.Running = fields[1] == "running"
		case "apiserver.version":
			// "container-apiserver version 1.3.1 (build: release, commit: abc)"
			for i, f := range fields {
				if f == "version" && i+1 < len(fields) {
					st.Version = fields[i+1]
				}
			}
		}
	}
	return st
}
