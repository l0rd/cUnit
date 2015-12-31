package build

import (
	"fmt"
	"os"
	"strings"

	"github.com/l0rd/docker-unit/build/commands"
	"github.com/l0rd/docker-unit/build/parser"
)

type TestBlock struct {
	Position      string
	DockerfileRef string
	Asserts       []parser.Command
	Ephemerals    []parser.Command
}

type TestStats struct {
	TotalNumberOfTests int
	NumberOfTestRan    int
	NumberOfTestPassed int
	NumberOfTestFailed int
}

type DockerfileTests struct {
	testBlocks []TestBlock
}

func newTester(testfilepath string) (*DockerfileTests, error) {

	dockerTestfile, err := os.Open(testfilepath)
	if err != nil {
		return nil, fmt.Errorf("unable to open DockerTestfile: %s", err)
	}
	defer dockerTestfile.Close()

	cmds, err := parser.Parse(dockerTestfile)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DockerTestfile: %s", err)
	}

	if len(cmds) == 0 {
		return nil, fmt.Errorf("no commands found in DockerTestfile")
	}

	t := &DockerfileTests{
		testBlocks: make([]TestBlock, 0),
	}

	currentTestBlock := &TestBlock{
		Asserts:    make([]parser.Command, 0),
		Ephemerals: make([]parser.Command, 0),
	}

	for i, fullcmd := range cmds {

		cmd, args := strings.ToUpper(fullcmd.Args[0]), fullcmd.Args[1:]

		if (i == 0) && ((cmd != commands.Before) && (cmd != commands.After)) {
			return nil, fmt.Errorf("Tests blocks should start with a %s or %s command (found %s instead)", commands.Before, commands.After, cmd)
		}

		if _, newTestBlock := commands.NewTestBlock[cmd]; newTestBlock {

			if len(currentTestBlock.Asserts) > 0 {
				t.testBlocks = append(t.testBlocks, *currentTestBlock)
			}

			currentTestBlock = &TestBlock{
				Position:      cmd,
				DockerfileRef: args[0],
				Asserts:       make([]parser.Command, 0),
				Ephemerals:    make([]parser.Command, 0),
			}

		} else {
			currentTestBlock.Asserts = append(currentTestBlock.Asserts, *fullcmd)
			ephemerals, err := Assert2Ephemeral(fullcmd)
			if err != nil {
				return nil, err
			}
			currentTestBlock.Ephemerals = append(currentTestBlock.Ephemerals, *ephemerals)
		}

	}

	t.testBlocks = append(t.testBlocks, *currentTestBlock)

	return t, nil
}

func Inject(cmds []*parser.Command, tests *DockerfileTests) ([]*parser.Command, error) {
	newCommands := make([]*parser.Command, 0)
	foundTestBlocks := 0

	for _, cmd := range cmds {

		cmdRef := toDockerfileRef(cmd)
		matchedBeforeTestBlocks := make([]TestBlock, 0)
		matchedAfterTestBlocks := make([]TestBlock, 0)

		for _, testBlock := range tests.testBlocks {
			if strings.HasPrefix(cmdRef, testBlock.DockerfileRef) {
				foundTestBlocks++
				if testBlock.Position == commands.Before {
					matchedBeforeTestBlocks = append(matchedBeforeTestBlocks, testBlock)
				}

				if testBlock.Position == commands.After {
					matchedAfterTestBlocks = append(matchedAfterTestBlocks, testBlock)
				}
			}
		}

		if len(matchedBeforeTestBlocks) > 1 || len(matchedAfterTestBlocks) > 1 {
			return nil, fmt.Errorf("Found more than one before/after test block that match a command (%s)", cmdRef)
		}

		if len(matchedBeforeTestBlocks) == 1 {
			for i, _ := range matchedBeforeTestBlocks[0].Ephemerals {
				newCommands = append(newCommands, &matchedBeforeTestBlocks[0].Ephemerals[i])
			}
		}

		newCommands = append(newCommands, cmd)

		if len(matchedAfterTestBlocks) == 1 {
			for i, _ := range matchedAfterTestBlocks[0].Ephemerals {
				newCommands = append(newCommands, &matchedAfterTestBlocks[0].Ephemerals[i])
			}
		}
	}

	if foundTestBlocks < len(tests.testBlocks) {
		return nil, fmt.Errorf("Some tests blocks could not be matched with Dockerfile instructions")
	}

	return newCommands, nil
}

func toDockerfileRef(command *parser.Command) string {
	return strings.ToUpper(strings.Join(command.Args, "_"))
}

func Assert2Ephemeral(command *parser.Command) (*parser.Command, error) {
	ephemeral := &parser.Command{Args: []string{"EPHEMERAL"}}

	if len(command.Args) < 2 {
		return nil, fmt.Errorf("Failed to convert an Assert command into an ephemeral: assert args is < 2 (assert=%s)", command.Args)
	}

	if command.Args[0] != commands.AssertTrue && command.Args[0] != commands.AssertFalse {
		return nil, fmt.Errorf("Asserts should start with %s or %s. Current assert starts with %s)", commands.AssertTrue, commands.AssertFalse, command.Args[0])
	}

	switch command.Args[1] {
	case "USER_EXISTS":
		if len(command.Args) != 3 {
			return nil, fmt.Errorf("Condition %s accept one and only one argument (found %d)", "USER_EXISTS", len(command.Args)-2)
		}
		ephemeral.Args = append(ephemeral.Args, "getent", "passwd", command.Args[2])

	case "FILE_EXISTS":
		if len(command.Args) != 3 {
			return nil, fmt.Errorf("Condition %s accept one and only one argument (found %d)", "FILE_EXISTS", len(command.Args)-2)
		}
		ephemeral.Args = append(ephemeral.Args, "bash", "-c")
		test := "test "
		if command.Args[0] == commands.AssertFalse {
			test += "! "
		}
		test += "-f " + command.Args[2]
		ephemeral.Args = append(ephemeral.Args, test)

	case "CURRENT_USER_IS":
		if len(command.Args) != 3 {
			return nil, fmt.Errorf("Condition %s accept one and only one argument (found %d)", "CURRENT_USER_IS", len(command.Args)-2)
		}
		ephemeral.Args = append(ephemeral.Args, "bash", "-c")
		test := "test "
		if command.Args[0] == commands.AssertFalse {
			test += "! "
		}
		test += "$(whoami) = \"" + command.Args[2] + "\""
		ephemeral.Args = append(ephemeral.Args, test)

	case "IS_INSTALLED":
		if len(command.Args) != 3 {
			return nil, fmt.Errorf("Condition %s accept one and only one argument (found %d)", "IS_INSTALLED", len(command.Args)-2)
		}
		ephemeral.Args = append(ephemeral.Args, "bash", "-c")
		test := ""
		if command.Args[0] == commands.AssertFalse {
			test += "! "
		}
		test += isInstalledGeneric(command.Args[2])
		ephemeral.Args = append(ephemeral.Args, test)

	case "IS_RUNNING":
		if len(command.Args) != 3 {
			return nil, fmt.Errorf("Condition %s accept one and only one argument (found %d)", "IS_INSTALLED", len(command.Args)-2)
		}
		ephemeral.Args = append(ephemeral.Args, "bash", "-c")
		test := "pidof "
		if command.Args[0] == commands.AssertFalse {
			test += "! "
		}
		test += command.Args[2]
		ephemeral.Args = append(ephemeral.Args, test)

	default:
		return nil, fmt.Errorf("Condition %s is not supported. Only %s, %s, %s and %s are currently supported. Please open an issue if you want to add support for it.", command.Args[1], "USER_EXISTS", "FILE_EXISTS", "CURRENT_USER_IS", "IS_INSTALLED", "IS_RUNNING")
	}

	return ephemeral, nil
}

func GetTotalNumberOfTests(tests *DockerfileTests) int {
	totalNumberOfTests := 0
	for _, testBlock := range tests.testBlocks {
		totalNumberOfTests += len(testBlock.Asserts)
	}
	return totalNumberOfTests
}

func PrintTestsStats(stats *TestStats) {
	fmt.Println()
	fmt.Println("----")
	fmt.Printf("Run %d tests: %d PASS and %d FAIL\n", stats.NumberOfTestRan, stats.NumberOfTestPassed, stats.NumberOfTestFailed)
	fmt.Println("----")
}

// func isInstalledDebian(packagename string) string {
//     return "\"$(dpkg-query -W -f='${Status}' " +
//            packagename +
//            ")\" = \"install ok installed\""
// }

func isInstalledGeneric(packagename string) string {
	return "command -v \"" + packagename + "\"  1>/dev/null 2>&1"
}
