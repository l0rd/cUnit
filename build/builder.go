package build

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	dockerclient2 "github.com/fsouza/go-dockerclient"
	"github.com/l0rd/docker-unit/build/commands"
	"github.com/l0rd/docker-unit/build/parser"
	"github.com/l0rd/docker-unit/build/util"
	"github.com/samalba/dockerclient"
)

type config struct {
	Cmd          []string
	Entrypoint   []string
	Env          []string
	ExposedPorts map[string]struct{}
	Labels       map[string]string
	User         string
	Volumes      map[string]struct{}
	WorkingDir   string
}

func (c *config) toDocker() *dockerclient.ContainerConfig {
	return &dockerclient.ContainerConfig{
		User:         c.User,
		ExposedPorts: c.ExposedPorts,
		Env:          c.Env,
		Cmd:          c.Cmd,
		Volumes:      c.Volumes,
		WorkingDir:   c.WorkingDir,
		Entrypoint:   c.Entrypoint,
		Labels:       c.Labels,
	}
}

type handlerFunc func(args []string, heredoc string) error

// Builder is able to build docker images from a local context directory, a
// Dockerfile, and a docker client connection.
type Builder struct {
	daemonURL           string
	tlsConfig           *tls.Config
	client              *dockerclient.DockerClient
	client2             *dockerclient2.Client
	contextDirectory    string
	dockerfilePath      string
	dockerTestfilePath  string
	dockerfileTests     *DockerfileTests
	dockerfileTestStats *TestStats
	repo, tag           string

	out io.Writer

	config              *config
	maintainer          string
	imageID             string
	containerID         string
	uncommitted         bool
	uncommittedCommands []string

	cache map[string]string

	handlers map[string]handlerFunc
}

// NewBuilder creates a new builder.
func NewBuilder(daemonURL string, tlsConfig *tls.Config, contextDirectory, dockerfilePath, repoTag string) (*Builder, error) {
	// Validate that the context directory exists.
	stat, err := os.Stat(contextDirectory)
	if err != nil {
		return nil, fmt.Errorf("unable to access build context directory: %s", err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("context must be a directory")
	}

	if dockerfilePath == "" {
		// Use Default path.
		dockerfilePath = filepath.Join(contextDirectory, "Dockerfile")
	}

	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, fmt.Errorf("unable to access build file: %s", err)
	}

	dockerTestfilePath := ""
	if _, err := os.Stat(dockerfilePath + "_test"); err == nil {
		dockerTestfilePath = dockerfilePath + "_test"
		fmt.Printf("Found test file: %s!\n\n", dockerTestfilePath)
	}

	// Validate the repository and tag.
	repo, tag := util.ParseRepositoryTag(repoTag)
	if repo != "" {
		if err := util.ValidateRepositoryName(repo); err != nil {
			return nil, fmt.Errorf("invalid repository name: %s", err)
		}
		if tag != "" {
			if err := util.ValidateTagName(tag); err != nil {
				return nil, fmt.Errorf("invalid tag: %s", err)
			}
		}
	}

	client, err := dockerclient.NewDockerClient(daemonURL, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize client: %s", err)
	}

	client2, err := dockerclient2.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize client2: %s", err)
	}

	b := &Builder{
		daemonURL:          daemonURL,
		tlsConfig:          tlsConfig,
		client:             client,
		client2:            client2,
		contextDirectory:   contextDirectory,
		dockerfilePath:     dockerfilePath,
		dockerTestfilePath: dockerTestfilePath,
		repo:               repo,
		tag:                tag,
		out:                os.Stdout,
		config: &config{
			Labels:       map[string]string{},
			ExposedPorts: map[string]struct{}{},
			Volumes:      map[string]struct{}{},
		},
	}

	// Register Dockerfile Directive Handlers
	b.handlers = map[string]handlerFunc{
		commands.Cmd:        b.handleCmd,
		commands.Copy:       b.handleCopy,
		commands.Entrypoint: b.handleEntrypoint,
		commands.Env:        b.handleEnv,
		commands.Ephemeral:  b.handleRun,
		commands.Expose:     b.handleExpose,
		commands.Extract:    b.handleExtract,
		commands.From:       b.handleFrom,
		commands.Label:      b.handleLabel,
		commands.Maintainer: b.handleMaintainer,
		commands.Run:        b.handleRun,
		commands.User:       b.handleUser,
		commands.Volume:     b.handleVolume,
		commands.Workdir:    b.handleWorkdir,

		// Not implemented for now:
		commands.Add:     b.handleAdd,
		commands.Onbuild: b.handleOnbuild,
	}

	if err := b.loadCache(); err != nil {
		return nil, fmt.Errorf("unable to load build cache: %s", err)
	}

	return b, nil
}

// Run executes the build process.
func (b *Builder) Run() error {

	// Parse the Dockerfile.
	dockerfile, err := os.Open(b.dockerfilePath)
	if err != nil {
		return fmt.Errorf("unable to open Dockerfile: %s", err)
	}
	defer dockerfile.Close()

	commands, err := parser.Parse(dockerfile)
	if err != nil {
		return fmt.Errorf("unable to parse Dockerfile: %s", err)
	}

	if len(commands) == 0 {
		return fmt.Errorf("no commands found in Dockerfile")
	}

	// Parse the DockerTestfile if it exists
	if b.dockerTestfilePath != "" {

		tester, err := newTester(b.dockerTestfilePath)

		if err != nil {
			return err
		}

		b.dockerfileTests = tester

		commands, err = Inject(commands, tester)

		if err != nil {
			return err
		}

		b.dockerfileTestStats = &TestStats{
			TotalNumberOfTests: GetTotalNumberOfTests(tester),
			NumberOfTestRan:    0,
			NumberOfTestPassed: 0,
			NumberOfTestFailed: 0,
		}

		defer printStats(*b)
	}

	for i, command := range commands {
		if err := b.dispatch(i, command); err != nil {
			return err
		}
	}

	// create container and commit if we need to (because of trailing
	// metadata directives).
	if b.uncommitted && !b.probeCache() {

		b.containerID, err = b.createContainer([]string{"/bin/sh", "-c"}, []string{"#(nop)"}, false)
		if err != nil {
			return fmt.Errorf("unable to create container: %s", err)
		}

		if err := b.commit(); err != nil {
			return fmt.Errorf("unable to commit container image: %s", err)
		}
	}

	imageName := b.imageID
	if b.repo != "" {
		if err := b.setTag(imageName, b.repo, b.tag); err != nil {
			return fmt.Errorf("unable to tag built image: %s", err)
		}

		imageName = b.repo
		if b.tag != "" {
			imageName = fmt.Sprintf("%s:%s", imageName, b.tag)
		}
	}

	fmt.Fprintf(b.out, "Successfully built %s\n", imageName)

	if err := b.dispatchPostBuildTests(); err != nil {
		return err
	}

	return nil
}

func (b *Builder) dispatch(stepNum int, command *parser.Command) error {
	cmd, args := strings.ToUpper(command.Args[0]), command.Args[1:]

	// FROM must be the first and only the first command.
	if (stepNum == 0) != (cmd == commands.From) {
		return fmt.Errorf("FROM must be the first Dockerfile command")
	}

	handler, exists := b.handlers[cmd]
	if !exists {
		return fmt.Errorf("unknown command: %q", cmd)
	}

	if _, ok := commands.ReplaceEnvAllowed[cmd]; ok {
		// Expand environment variables in the arguments.
		for i, arg := range args {
			arg, err := processShellWord(arg, b.config.Env)
			if err != nil {
				return err
			}

			args[i] = arg
		}
	}

	// Print the current step.
	commandStr := makeCommandString(cmd, args...)

	fmt.Fprintf(b.out, "Step %d: %s\n", stepNum, commandStr)

	if cmd != commands.Ephemeral {
		b.uncommitted = true
		b.uncommittedCommands = append(b.uncommittedCommands, commandStr)
	} else {
		// must set uncommitted = false
		// to make it clear that it's an
		// EPHEMERAL command when handler()
		// is called
		b.uncommitted = false
		b.dockerfileTestStats.NumberOfTestRan += 1
	}

	if err := handler(args, command.Heredoc); err != nil {
		if cmd == commands.Ephemeral {
			b.dockerfileTestStats.NumberOfTestFailed += 1
		}
		return err
	}

	if cmd == commands.Ephemeral {
		b.dockerfileTestStats.NumberOfTestPassed += 1
	}

	// We may not need to commit now but we should if the current command may
	// have modified the filesystem. `b.uncommitted` will be set back to false
	// if there was a cache hit.
	if _, needCommit := commands.FilesystemModifierCommands[cmd]; needCommit && b.uncommitted {
		if err := b.commit(); err != nil {
			return fmt.Errorf("unable to commit container image: %s", err)
		}
	}

	if b.containerID != "" {
		if err := b.client.RemoveContainer(b.containerID, true, true); err != nil {
			return fmt.Errorf("unable to remove container: %s", err)
		}
		b.containerID = ""
		fmt.Fprintf(b.out, " removed temporary container %s\n", b.containerID)
	}

	return nil
}

// makeCommandString returns a printable form of the command and arguments with
// arguments quoted if necessary.
func makeCommandString(cmd string, args ...string) string {
	quotedArgs := make([]string, len(args))

	for i, arg := range args {
		if strings.ContainsAny(arg, "<#'\" \f\n\r\t\v\\") {
			arg = strconv.Quote(arg)
		}
		quotedArgs[i] = arg
	}

	return fmt.Sprintf("%s %s", cmd, strings.Join(quotedArgs, " "))
}

func printStats(b Builder) {
	fmt.Printf(fmt.Sprintln() +
		fmt.Sprintln("----") +
		b.TestsStatsString() +
		fmt.Sprintln("\n----"))
}
