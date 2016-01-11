package build

import (
	"os"
	"testing"

	"github.com/l0rd/docker-unit/build/parser"
)

// to test that run the following command
//    GOPATH="$project_dir/Godeps/_workspace:$HOME/GitHub/docker-unit" go test -test.v github.com/l0rd/docker-unit/build

func TestNewTester(t *testing.T) {

	const testfile = "testfile"

	tests, err := newTester(testfile)

	if err != nil {
		t.Error("Error creating newTester.", err)
		return
	}

	if tests == nil {
		t.Errorf("Failed to test file %s", tests)
	}

	if blockNum := len(tests.testBlocks); blockNum != 8 {
		t.Errorf("Expected 8 blocks, found %d", blockNum)
	}
}

func TestInjection(t *testing.T) {

	expectedEphemerals := []parser.Command{
		{Args: []string{"FROM", "tomcat:8.0.28-jre8"}},
		{Args: []string{"RUN", "useradd", "-d", "/home/mario", "-m", "-s", "/bin/bash", "mario"}},
		{Args: []string{"EPHEMERAL", "getent", "passwd", "mario"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "test -f /home/mario/.profile"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "test ! -f /usr/local/tomcat/webapps/words"}},
		{Args: []string{"COPY", "words", "/usr/local/tomcat/webapps/"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "test -f /usr/local/tomcat/webapps/words"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "test $(whoami) = \"root\""}},
		{Args: []string{"USER", "mario"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "test $(whoami) = \"mario\""}},
		{Args: []string{"RUN", "bash", "-c", "echo bar >> /tmp/foo.txt"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "grep -q bar /tmp/foo.txt"}},
		//		{Args: []string{"EPHEMERAL",  "bash", "-c", "test ! \"$(dpkg-query -W -f='${Status}' vim)\" = \"install ok installed\""}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "! command -v \"vim\"  1>/dev/null 2>&1"}},
		{Args: []string{"RUN", "apt-get", "update", "&&", "apt-get", "install", "-y", "vim"}},
		{Args: []string{"EPHEMERAL", "bash", "-c", "command -v \"vim\"  1>/dev/null 2>&1"}},
		//		{Args: []string{"EPHEMERAL",  "bash", "-c", "test \"$(dpkg-query -W -f='${Status}' vim)\" = \"install ok installed\""}},
		{Args: []string{"CMD", "catalina.sh", "run"}},
	}

	const (
		testfilepath   = "testfile"
		dockerfilepath = "dockerfile"
	)

	dockerfile, err := os.Open(dockerfilepath)
	if err != nil {
		t.Errorf("unable to open Dockerfile: %s", err)
		return
	}

	defer dockerfile.Close()

	commands, err := parser.Parse(dockerfile)
	if err != nil {
		t.Errorf("unable to parse Dockerfile: %s", err)
		return
	}

	if len(commands) == 0 {
		t.Errorf("no commands found in Dockerfile")
		return
	}

	tests, err := newTester(testfilepath)
	if err != nil {
		t.Error("Error createing newTester")
		return
	}

	newCommands, err := Inject(commands, tests)
	if err != nil {
		t.Errorf("Error injecting tests blocks into dockerfile: %s", err)
		return
	}

	// printCommands(t, newCommands)
	if len(expectedEphemerals) != len(newCommands) {
		t.Errorf("Expected newCommands size was %d but found %d", len(expectedEphemerals), len(newCommands))
		return
	}

	for i, cmd := range newCommands {
		cmdargs := cmd.Args
		if len(cmdargs) != len(expectedEphemerals[i].Args) {
			t.Errorf("Expected len of command[%d] was %d. Actual len value is %d instead", i, len(expectedEphemerals[i].Args), len(cmdargs))
			return
		}

		for j, arg := range expectedEphemerals[i].Args {
			if arg != cmdargs[j] {
				t.Errorf("Expected command[%d].arg[%d] was %s but found %s instead", i, j, arg, cmdargs[j])
				return
			}
		}
	}
}

func TestAssert2Ephemeral(t *testing.T) {
	cases := []struct {
		in, expected parser.Command
	}{
		{
			parser.Command{Args: []string{"ASSERT_TRUE", "USER_EXISTS", "tomcat"}},
			parser.Command{Args: []string{"EPHEMERAL", "getent", "passwd", "tomcat"}},
		},
	}

	for _, c := range cases {
		actual, err := Assert2Ephemeral(&c.in)

		if err != nil {
			t.Errorf("Failed to transform %s to and EPHEMERAL instruction", c.in.Args)
			return
		}

		if len(actual.Args) != len(c.expected.Args) {
			t.Errorf("Assert2Ephemeral(%q) == %q, expected %q", c.in, actual, c.expected)
		}

		for i, arg := range actual.Args {
			if arg != c.expected.Args[i] {
				t.Errorf("Assert2Ephemeral(%q) == %q, expected %q", c.in, actual, c.expected)
			}
		}
	}
}

func printCommands(t *testing.T, commands []*parser.Command) {

	for _, cmd := range commands {
		t.Log(cmd.Args)
	}

}

func printTests(t *testing.T, tests *DockerfileTests) {

	for _, test := range tests.testBlocks {
		for _, assert := range test.Asserts {
			t.Log(assert.Args)
		}
	}

}
