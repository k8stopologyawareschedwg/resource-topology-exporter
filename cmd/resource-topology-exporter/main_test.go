package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "github.com/stretchr/testify/suite"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
)

const (
	node      = "TEST_NODE"
	namespace = "TEST_NS"
	pod       = "TEST_POD"
	container = "TEST_CONT"
)

var (
	update = flag.Bool("update", false, "update golden files")
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing context
type ArgsParseTestSuite struct {
	Suite
	baseDir     string
	testDataDir string
}

// SetupTest run before each test
func (s *ArgsParseTestSuite) SetupTest() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		s.Fail("Cannot retrieve tests directory")
	}

	s.baseDir = filepath.Dir(file)
	s.testDataDir = filepath.Clean(filepath.Join(s.baseDir, "..", "..", "test", "data"))

	os.Setenv("NODE_NAME", node)
	os.Setenv("REFERENCE_NAMESPACE", namespace)
	os.Setenv("REFERENCE_POD_NAME", pod)
	os.Setenv("REFERENCE_CONTAINER_NAME", container)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestArgsParseSuite(t *testing.T) {
	Run(t, new(ArgsParseTestSuite))
}

// All methods that begin with "Test" are run as tests within a suite.
func (s *ArgsParseTestSuite) TestArgsParse() {
	Convey("when parsing command line arguments", s.T(), func() {
		Convey("contains correct values in flags provided", func() {
			pArgs, err := parseArgs("--oneshot", "--no-publish")
			So(err, ShouldBeNil)
			So(pArgs.NRTupdater.Oneshot, ShouldBeTrue)
			So(pArgs.NRTupdater.NoPublish, ShouldBeTrue)
		})

		Convey("must parse custom flags properly", func() {
			pArgs, err := parseArgs("--kubelet-state-dir=dir1 dir2 dir3", "--reference-container=ns/pod/cont")
			So(err, ShouldBeNil)
			So(pArgs.RTE.KubeletStateDirs, ShouldResemble, []string{"dir1", "dir2", "dir3"})
			So(pArgs.RTE.ReferenceContainer, ShouldResemble, &podrescli.ContainerIdent{Namespace: "ns", PodName: "pod", ContainerName: "cont"})
		})

		Convey("should have the following default values", func() {
			pArgs, err := parseArgs()
			So(err, ShouldBeNil)

			pArgsAsJson, err := pArgs.ToJson()
			So(err, ShouldBeNil)

			// s.T().Name() looks like TestArgsParseSuite/TestArgsParse, so remove the first part
			golden := filepath.Join(s.testDataDir, fmt.Sprintf("%s.default.json", strings.Split(s.T().Name(), "/")[1]))
			if *update {
				err = ioutil.WriteFile(golden, pArgsAsJson, 0644)
				So(err, ShouldBeNil)
			}
			expected, err := ioutil.ReadFile(golden)
			So(err, ShouldBeNil)

			// compare the default values from the file with the output from the parseArgs function
			So(string(pArgsAsJson), ShouldResemble, string(expected))
		})
	})
}
