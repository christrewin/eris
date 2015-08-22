package chains

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/eris-ltd/eris-cli/data"
	def "github.com/eris-ltd/eris-cli/definitions"
	ini "github.com/eris-ltd/eris-cli/initialize"
	"github.com/eris-ltd/eris-cli/loaders"
	"github.com/eris-ltd/eris-cli/perform"
	"github.com/eris-ltd/eris-cli/services"
	"github.com/eris-ltd/eris-cli/util"
	"github.com/eris-ltd/eris-cli/version"

	"github.com/eris-ltd/eris-cli/Godeps/_workspace/src/github.com/eris-ltd/common/go/common"
	"github.com/eris-ltd/eris-cli/Godeps/_workspace/src/github.com/eris-ltd/common/go/log"
)

var erisDir string = path.Join(os.TempDir(), "eris")
var chainName string = "my_tests" // :(
var hash string

var DEAD bool // XXX: don't double panic (TODO: Flushing twice blocks)

func fatal(t *testing.T, err error) {
	if !DEAD {
		log.Flush()
		testsTearDown()
		DEAD = true
		panic(err)
	}
}

func TestMain(m *testing.M) {
	var logLevel log.LogLevel
	var err error

	//logLevel = 0
	// logLevel = 1
	logLevel = 3

	log.SetLoggers(logLevel, os.Stdout, os.Stderr)

	testsInit()
	logger.Infoln("Test init completed. Starting main test sequence now.\n")

	exitCode := m.Run()

	logger.Infoln("Commensing with Tests Tear Down.")
	err = testsTearDown()
	if err != nil {
		logger.Errorln(err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func TestKnownChain(t *testing.T) {
	do := def.NowDo()
	ifExit(ListKnown(do))

	k := strings.Split(do.Result, "\n") // tests output formatting.

	if k[0] != chainName {
		logger.Debugf("Result =>\t\t%s\n", do.Result)
		ifExit(fmt.Errorf("Found a chain definition file. Something is wrong."))
	}
}

func TestChainGraduate(t *testing.T) {
	do := def.NowDo()
	do.Name = chainName
	logger.Infof("Graduating chain (from tests) ==>\t%s\n", do.Name)
	if err := GraduateChain(do); err != nil {
		fatal(t, err)
	}

	srvDef, err := loaders.LoadServiceDefinition(chainName, false, 1)
	if err != nil {
		fatal(t, err)
	}

	vers := strings.Join(strings.Split(version.VERSION, ".")[0:2], ".")
	image := "eris/erisdb:" + vers
	if srvDef.Service.Image != image {
		fatal(t, fmt.Errorf("FAILURE: improper service image on GRADUATE. expected: %s\tgot: %s\n", image, srvDef.Service.Image))
	}

	if srvDef.Service.Command != loaders.ErisChainStart {
		fatal(t, fmt.Errorf("FAILURE: improper service command on GRADUATE. expected: %s\tgot: %s\n", loaders.ErisChainStart, srvDef.Service.Command))
	}

	if !srvDef.Service.AutoData {
		fatal(t, fmt.Errorf("FAILURE: improper service autodata on GRADUATE. expected: %t\tgot: %t\n", true, srvDef.Service.AutoData))
	}

	if len(srvDef.ServiceDeps.Dependencies) != 1 {
		fatal(t, fmt.Errorf("FAILURE: improper service deps on GRADUATE. expected: [\"keys\"]\tgot: %s\n", srvDef.ServiceDeps))
	}
}

func TestLoadChainDefinition(t *testing.T) {
	var e error
	logger.Infof("Load chain def (from tests) =>\t%s\n", chainName)
	chn, e := loaders.LoadChainDefinition(chainName, false, 1)
	if e != nil {
		fatal(t, e)
	}

	if chn.Service.Name != chainName {
		fatal(t, fmt.Errorf("FAILURE: improper service name on LOAD. expected: %s\tgot: %s", chainName, chn.Service.Name))
	}

	if !chn.Service.AutoData {
		fatal(t, fmt.Errorf("FAILURE: data_container not properly read on LOAD."))
	}

	if chn.Operations.DataContainerName == "" {
		fatal(t, fmt.Errorf("FAILURE: data_container_name not set."))
	}
}

func TestStartKillChain(t *testing.T) {
	testStartChain(t, chainName)
	testKillChain(t, chainName)
}

// eris chains new --dir
func TestChainsNewDir(t *testing.T) {
	chainID := "mynewchain"
	myDir := path.Join(common.DataContainersPath, chainID)
	if err := os.MkdirAll(myDir, 0700); err != nil {
		fatal(t, err)
	}
	contents := []byte("this is a file in the directory\n")
	if err := ioutil.WriteFile(path.Join(myDir, "file.file"), contents, 0600); err != nil {
		fatal(t, err)
	}

	do := def.NowDo()
	do.GenesisFile = path.Join(common.BlockchainsPath, "config", "default", "genesis.json")
	do.Name = chainID
	do.Path = myDir
	do.Operations.ContainerNumber = 1
	logger.Infof("Creating chain (from tests) =>\t%s\n", do.Name)
	ifExit(NewChain(do))

	// remove the data container
	defer data.RmData(do)

	// verify the contents
	do.Name = util.DataContainersName(do.Name, do.Operations.ContainerNumber)
	oldWriter := util.GlobalConfig.Writer
	newWriter := new(bytes.Buffer)
	util.GlobalConfig.Writer = newWriter
	//args := []string{fmt.Sprintf("cat /home/eris/.eris/blockchains/%s/file.file", chainID)}
	args := []string{"cat", fmt.Sprintf("/home/eris/.eris/blockchains/%s/file.file", chainID)}
	if err := perform.DockerRunVolumesFromContainer(do.Name, false, args); err != nil {
		fatal(t, err)
	}
	util.GlobalConfig.Writer = oldWriter
	result := newWriter.Bytes()
	result = result[:len(result)-2]
	contents = contents[:len(contents)-1]
	if string(result) != string(contents) {
		fatal(t, fmt.Errorf("file not faithfully copied. Got: %s \n Expected: %s", result, contents))
	}
}

func TestLogsChain(t *testing.T) {
	testStartChain(t, chainName)
	defer testKillChain(t, chainName)

	do := def.NowDo()
	do.Name = chainName
	do.Follow = false
	do.Tail = "all"
	logger.Infof("Get chain logs (from tests) =>\t%s:%s\n", do.Name, do.Tail)
	e := LogsChain(do)
	if e != nil {
		fatal(t, e)
	}
}

func TestUpdateChain(t *testing.T) {
	testStartChain(t, chainName)
	defer testKillChain(t, chainName)

	do := def.NowDo()
	do.Name = chainName
	do.SkipPull = true
	logger.Infof("Updating chain (from tests) =>\t%s\n", do.Name)
	e := UpdateChain(do)
	if e != nil {
		fatal(t, e)
	}

	testExistAndRun(t, chainName, true, true)
}

func TestInspectChain(t *testing.T) {
	testStartChain(t, chainName)
	defer testKillChain(t, chainName)

	do := def.NowDo()
	do.Name = chainName
	do.Args = []string{"name"}
	do.Operations.ContainerNumber = 1
	logger.Debugf("Inspect chain (via tests) =>\t%s:%v\n", chainName, do.Args)
	e := InspectChain(do)
	if e != nil {
		fatal(t, fmt.Errorf("Error inspecting chain =>\t%v\n", e))
	}
	// log.SetLoggers(0, os.Stdout, os.Stderr)
}

func TestRenameChain(t *testing.T) {
	oldName := chainName
	newName := "niahctset"
	testStartChain(t, oldName)
	defer testKillChain(t, oldName)

	do := def.NowDo()
	do.Name = oldName
	do.NewName = newName
	logger.Infof("Renaming chain (from tests) =>\t%s:%s\n", do.Name, do.NewName)
	e := RenameChain(do)
	if e != nil {
		fatal(t, e)
	}

	testExistAndRun(t, newName, true, true)

	do = def.NowDo()
	do.Name = newName
	do.NewName = chainName
	logger.Infof("Renaming chain (from tests) =>\t%s:%s\n", do.Name, do.NewName)
	e = RenameChain(do)
	if e != nil {
		fatal(t, e)
	}

	testExistAndRun(t, chainName, true, true)
}

// TODO: finish this....
// func TestServiceWithChainDependencies(t *testing.T) {
// 	do := definitions.NowDo()
// 	do.Name = "keys"
// 	do.Args = []string{"eris/keys"}
// 	err := services.NewService(do)
// 	if err != nil {
// 		logger.Errorln(err)
// 		t.FailNow()
// 	}

// 	services.TestCatService(t)

// }

func TestRmChain(t *testing.T) {
	testStartChain(t, chainName)

	do := def.NowDo()
	do.Args, do.Rm, do.RmD = []string{"keys"}, true, true
	logger.Infof("Removing keys (from tests) =>\n%s\n", do.Name)
	if e := services.KillService(do); e != nil {
		fatal(t, e)
	}

	do = def.NowDo()
	do.Name, do.Rm, do.RmD = chainName, false, false
	logger.Infof("Stopping chain (from tests) =>\t%s\n", do.Name)
	if e := KillChain(do); e != nil {
		fatal(t, e)
	}
	testExistAndRun(t, chainName, true, false)

	do = def.NowDo()
	do.Name = chainName
	do.RmD = true
	logger.Infof("Removing chain (from tests) =>\n%s\n", do.Name)
	e := RmChain(do)
	if e != nil {
		fatal(t, e)
	}

	testExistAndRun(t, chainName, false, false)
}

//------------------------------------------------------------------
// testing utils

func testStartChain(t *testing.T, chain string) {
	do := def.NowDo()
	do.Name = chain
	do.Operations.ContainerNumber = 1
	logger.Infof("Starting chain (from tests) =>\t%s\n", do.Name)
	e := StartChain(do)
	if e != nil {
		logger.Errorln(e)
		fatal(t, nil)
	}
	testExistAndRun(t, chain, true, true)
}

func testKillChain(t *testing.T, chain string) {
	// log.SetLoggers(2, os.Stdout, os.Stderr)
	testExistAndRun(t, chain, true, true)

	do := def.NowDo()
	do.Args, do.Rm, do.RmD = []string{"keys"}, true, true
	logger.Infof("Removing keys (from tests) =>\n%s\n", do.Name)
	if e := services.KillService(do); e != nil {
		fatal(t, e)
	}

	do = def.NowDo()
	do.Name, do.Rm, do.RmD = chain, true, true
	logger.Infof("Stopping chain (from tests) =>\t%s\n", do.Name)
	if e := KillChain(do); e != nil {
		fatal(t, e)
	}
	testExistAndRun(t, chain, false, false)
}

func testExistAndRun(t *testing.T, chainName string, toExist, toRun bool) {
	var exist, run bool
	logger.Infof("\nTesting whether (%s) is running? (%t) and existing? (%t)\n", chainName, toRun, toExist)
	chainName = util.ChainContainersName(chainName, 1) // not worried about containerNumbers, deal with multiple containers in services tests

	do := def.NowDo()
	do.Quiet = true
	do.Args = []string{"testing"}
	if err := ListExisting(do); err != nil {
		fatal(t, err)
	}
	res := strings.Split(do.Result, "\n")
	for _, r := range res {
		logger.Debugf("Existing =>\t\t\t%s\n", r)
		if r == util.ContainersShortName(chainName) {
			exist = true
		}
	}

	do = def.NowDo()
	do.Quiet = true
	do.Args = []string{"testing"}
	if err := ListRunning(do); err != nil {
		fatal(t, err)
	}
	res = strings.Split(do.Result, "\n")
	for _, r := range res {
		logger.Debugf("Running =>\t\t\t%s\n", r)
		if r == util.ContainersShortName(chainName) {
			run = true
		}
	}

	if toExist != exist {
		if toExist {
			logger.Infof("Could not find an existing =>\t%s\n", chainName)
		} else {
			logger.Infof("Found an existing instance of %s when I shouldn't have\n", chainName)
		}
		fatal(t, nil)
	}

	if toRun != run {
		if toRun {
			logger.Infof("Could not find a running =>\t%s\n", chainName)
		} else {
			logger.Infof("Found a running instance of %s when I shouldn't have\n", chainName)
		}
		fatal(t, nil)
	}

	logger.Debugln("")
}

func testsInit() error {
	var err error
	// TODO: make a reader/pipe so we can see what is written from tests.
	util.GlobalConfig, err = util.SetGlobalObject(os.Stdout, os.Stderr)
	if err != nil {
		ifExit(fmt.Errorf("TRAGIC. Could not set global config.\n"))
	}

	// common is initialized on import so
	// we have to manually override these
	// variables to ensure that the tests
	// run correctly.
	util.ChangeErisDir(erisDir)

	// init dockerClient
	util.DockerConnect(false, "eris")

	// this dumps the ipfs service def into the temp dir which
	// has been set as the erisRoot
	if err := ini.Initialize(true, false); err != nil {
		ifExit(fmt.Errorf("TRAGIC. Could not initialize the eris dir.\n"))
	}

	// lay a chain service def
	testNewChain(chainName)

	return nil
}

func testNewChain(chain string) {
	do := def.NowDo()
	do.GenesisFile = path.Join(common.BlockchainsPath, "config", "default", "genesis.json")
	do.Name = chain
	do.Operations.ContainerNumber = 1
	logger.Infof("Creating chain (from tests) =>\t%s\n", do.Name)
	ifExit(NewChain(do)) // configFile and dir are not needed for the tests.

	// remove the data container
	ifExit(data.RmData(do))
}

func testsTearDown() error {
	return os.RemoveAll(erisDir)
}

func ifExit(err error) {
	if err != nil {
		logger.Errorln(err)
		log.Flush()
		testsTearDown()
		os.Exit(1)
	}
}
