package docker

import (
	"bufio"
	"github.com/pkg/errors"
	"io"
	"os/exec"
	"strings"
)

func CheckImageExists(name string, useSudo bool) error {

	cmdParts := []string {"docker", "pull", name}
	if useSudo {
		cmdParts = append([]string{"sudo"}, cmdParts...)
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	stdout, err := cmd.StdoutPipe()
	stderr, err := cmd.StderrPipe()
	// cmd.Env = ctx.GetMinikubeDockerEnv()
	if err != nil {
		return err
	}
	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)

	if err = cmd.Start(); err != nil {
		return err
	}

	defer cmd.Process.Kill()

	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.Contains(line, "Pulling from") {
			return nil
		}
		if strings.Contains(line, "Error") {
			return errors.New(line)
		}
	}

	if err = scanner.Err(); err != nil {
		return err
	}

	_ = cmd.Process.Kill()

	state, err := cmd.Process.Wait()
	if err != nil {
		return err
	}

	if !state.Success() {
		return errors.Errorf("Pull failed: %s\n%s", state.String(), strings.Join(lines, "\n"))
	}

	return nil
}
