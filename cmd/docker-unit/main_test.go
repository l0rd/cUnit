package main

import (
	"encoding/json"
	"github.com/l0rd/docker-unit/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type dockerUnitTestCase struct {
	Dockerfile     string
	Dockertestfile string
	Shouldfail     bool
	Stats          string
}

const testcasedir = "./testcases/"

func TestDockerUnit(t *testing.T) {

	docker := dockerClientConnection{}

	getDockerClientConnection(&docker)
    
    testcases, err := parseTestCases(testcasedir)
    
    if err != nil {
        t.Fatalf("Error parsing testscases in folder \"%s\":%s", testcasedir, err)
    }

	for _, testcase := range testcases {

		dockerfile, dockertestfile, err := createTempFiles(testcase.Dockerfile, testcase.Dockertestfile)
		if err != nil {
			t.Fatalf("unable create temp files: %s", err)
		}

		defer dockerfile.Close()
		defer dockertestfile.Close()

		builder, err := build.NewBuilder(docker.daemonURL, docker.tlsConfig, ".", dockerfile.Name(), "")
		if err != nil {
			t.Fatalf("unable to initialize builder: %s", err)
		}

		err = builder.Run()
		if testcase.Shouldfail && err == nil {
			t.Fatal("Test should have failed but reterned zero instead")
		}

		if !testcase.Shouldfail && err != nil {
			t.Fatal(err)
		}

		stats := builder.TestsStatsString()

		if stats != testcase.Stats {
			t.Fatalf("Expected \"%s\", found \"%s\"", testcase.Stats, stats)
		}
	}
}

func parseTestCases(testcasedir string) (testcases []dockerUnitTestCase, err error) {

	testcasejsons := getTestFiles(testcasedir)

	for _, testcasejson := range testcasejsons {

		var currentTestCases []dockerUnitTestCase

		testcasefile, err := os.Open(testcasejson)
		if err != nil {
			return nil, err
		}

		jsonParser := json.NewDecoder(testcasefile)
		if err = jsonParser.Decode(&currentTestCases); err != nil {
			return nil, err
		}

		testcases = append(testcases, currentTestCases...)

	}

	return testcases, nil

}

func getTestFiles(searchDir string) (fileList []string) {
	filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".json") {
			fileList = append(fileList, path)
		}
		return nil
	})
	return fileList
}

func createTempFiles(dockerfileContent string, dockertestfileContent string) (dockerfile *os.File, dockertestfile *os.File, err error) {

	dockerfile, err = ioutil.TempFile("", "docker-unit-test")
	if err != nil {
		return nil, nil, err
	}
	dockerfile.WriteString(dockerfileContent)

	dockertestfile, err = os.Create(dockerfile.Name() + "_test")
	if err != nil {
		return nil, nil, err
	}
	dockertestfile.WriteString(dockertestfileContent)
	return dockerfile, dockertestfile, nil
}
