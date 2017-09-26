// +build integration

/*
Copyright (C) 2017 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/godog"
	"github.com/DATA-DOG/godog/gherkin"
	"gopkg.in/yaml.v2"

	"github.com/minishift/minishift/pkg/minikube/constants"
	testProxy "github.com/minishift/minishift/test/integration/proxy"
	"github.com/minishift/minishift/test/integration/util"
)

var (
	minishift       *Minishift
	minishiftArgs   string
	minishiftBinary string

	testDir string
	isoName string

	// Godog options
	godogFormat              string
	godogTags                string
	godogShowStepDefinitions bool
	godogStopOnFailure       bool
	godogNoColors            bool
	godogPaths               string
)

func TestMain(m *testing.M) {
	parseFlags()

	if godogTags != "" {
		godogTags += "&&"
	}
	runner := util.MinishiftRunner{CommandPath: minishiftBinary}
	if runner.IsCDK() {
		godogTags += "~minishift-only"
		isoName = "rhel"
		fmt.Println("Test run using CDK binary with RHEL iso.")
	} else {
		godogTags += "~cdk-only"
		isoUrl := os.Getenv("MINISHIFT_ISO_URL")
		switch isoUrl {
		case "b2d":
			fmt.Println("Test run using Boot2Docker iso image.")
			isoName = "b2d"
		case "centos":
			fmt.Println("Test run using CentOS iso image.")
			isoName = "centos"
		case "":
			fmt.Println("Test run using Boot2Docker iso image.")
			isoName = "b2d"
		default:
			fmt.Print("Using full path for iso image. ")
			isoName = determineIsoFromFile(isoUrl)
		}
	}

	status := godog.RunWithOptions("minishift", func(s *godog.Suite) {
		FeatureContext(s)
	}, godog.Options{
		Format:              godogFormat,
		Paths:               strings.Split(godogPaths, ","),
		Tags:                godogTags,
		ShowStepDefinitions: godogShowStepDefinitions,
		StopOnFailure:       godogStopOnFailure,
		NoColors:            godogNoColors,
	})

	if st := m.Run(); st > status {
		status = st
	}
	os.Exit(status)
}

func parseFlags() {
	flag.StringVar(&minishiftArgs, "minishift-args", "", "Arguments to pass to minishift")
	flag.StringVar(&minishiftBinary, "binary", "", "Path to minishift binary")

	flag.StringVar(&testDir, "test-dir", "", "Path to the directory in which to execute the tests")

	flag.StringVar(&godogFormat, "format", "pretty", "Sets which format godog will use")
	flag.StringVar(&godogTags, "tags", "", "Tags for godog test")
	flag.BoolVar(&godogShowStepDefinitions, "definitions", false, "")
	flag.BoolVar(&godogStopOnFailure, "stop-on-failure ", false, "Stop when failure is found")
	flag.BoolVar(&godogNoColors, "no-colors", false, "Disable colors in godog output")
	flag.StringVar(&godogPaths, "paths", "./features", "")

	flag.Parse()
}

func determineIsoFromFile(isoUrl string) string {
	var isoName string
	if matched, _ := regexp.MatchString(".*centos7\\.iso", isoUrl); matched {
		fmt.Println("CentOS variant was assumed from the filename of ISO.")
		isoName = "centos"
	} else if matched, _ := regexp.MatchString(".*b2d\\.iso", isoUrl); matched {
		fmt.Println("Boot2docker variant was assumed from the filename of ISO.")
		isoName = "b2d"
	} else {
		fmt.Println("Can't assume ISO variant from its filename. Will use Boot2Docker. To avoid this situation please name your ISO to end with 'b2d.iso' or 'centos7.iso'.")
		isoName = "b2d"
	}
	return isoName
}

func FeatureContext(s *godog.Suite) {
	runner := util.MinishiftRunner{
		CommandArgs: minishiftArgs,
		CommandPath: minishiftBinary}

	minishift = &Minishift{runner: runner}

	// steps to execute `minishift` commands
	s.Step(`^Minishift (?:has|should have) state "(Does Not Exist|Running|Stopped)"$`,
		minishift.shouldHaveState)
	s.Step(`^executing "minishift (.*)"$`,
		minishift.executingMinishiftCommand)
	s.Step(`^executing "minishift (.*)" (succeeds|fails)$`,
		executingMinishiftCommandSucceedsOrFails)
	s.Step(`^([^"]*) of command "minishift (.*)" (is equal|is not equal) to "(.*)"$`,
		commandReturnEquals)
	s.Step(`^([^"]*) of command "minishift (.*)" (contains|does not contain) "(.*)"$`,
		commandReturnContains)

	// steps to execute `oc` commands
	s.Step(`^executing "oc (.*)" retrying (\d+) times with wait period of (\d+) seconds$`,
		minishift.executingRetryingTimesWithWaitPeriodOfSeconds)
	s.Step(`^executing "oc (.*)"$`,
		minishift.executingOcCommand)
	s.Step(`^executing "oc (.*)" (succeeds|fails)$`,
		executingOcCommandSucceedsOrFails)

	// steps for scenario variables
	s.Step(`^setting scenario variable "(.*)" to the stdout from executing "oc (.*)"$`,
		minishift.setVariableExecutingOcCommand)
	s.Step(`^scenario variable "(.*)" should not be empty$`,
		variableShouldNotBeEmpty)

	// steps for rollout check
	s.Step(`^services "([^"]*)" rollout successfully$`,
		minishift.rolloutServicesSuccessfully)

	// steps for proxying
	s.Step(`^user starts proxy server and sets MINISHIFT_HTTP_PROXY variable$`,
		testProxy.SetProxy)
	s.Step(`^user stops proxy server and unsets MINISHIFT_HTTP_PROXY variable$`,
		testProxy.UnsetProxy)
	s.Step(`^proxy log should contain "(.*)"$`,
		proxyLogShouldContain)
	s.Step(`^proxy log should contain$`,
		proxyLogShouldContainContent)

	// steps to verify `stdout`, `stderr` and `exitcode` of commands executed
	s.Step(`^(stdout|stderr|exitcode) should contain "(.*)"$`,
		commandReturnShouldContain)
	s.Step(`^(stdout|stderr|exitcode) should not contain "(.*)"$`,
		commandReturnShouldNotContain)
	s.Step(`^(stdout|stderr|exitcode) should contain$`,
		commandReturnShouldContainContent)
	s.Step(`^(stdout|stderr|exitcode) should not contain$`,
		commandReturnShouldNotContainContent)
	s.Step(`^(stdout|stderr|exitcode) should equal "(.*)"$`,
		commandReturnShouldEqual)
	s.Step(`^(stdout|stderr|exitcode) should equal$`,
		commandReturnShouldEqualContent)
	s.Step(`^(stdout|stderr|exitcode) should be empty$`,
		commandReturnShouldBeEmpty)
	s.Step(`^(stdout|stderr|exitcode) should not be empty$`,
		commandReturnShouldNotBeEmpty)
	s.Step(`^(stdout|stderr|exitcode) should be valid (.*)$`,
		shouldBeInValidFormat)
	// steps for matching stdout, stderr or exitcode with regular expression
	s.Step(`^(stdout|stderr|exitcode) should match "(.*)"$`,
		commandReturnShouldMatchRegex)
	s.Step(`^(stdout|stderr|exitcode) should not match "(.*)"$`,
		commandReturnShouldNotMatchRegex)
	s.Step(`^(stdout|stderr|exitcode) should match$`,
		commandReturnShouldMatchRegexContent)
	s.Step(`^(stdout|stderr|exitcode) should not match$`,
		commandReturnShouldNotMatchRegexContent)

	// step for HTTP requests for minishift web console
	s.Step(`^(body|status code) of HTTP request to "([^"]*)" (?:|at "([^"]*)" )(contains|is equal to) "(.*)"$`,
		verifyHTTPResponse)

	// step for HTTP requests for accessing application
	s.Step(`^(body|status code) of HTTP request to "([^"]*)" of service "([^"]*)" in namespace "([^"]*)" (contains|is equal to) "(.*)"$`,
		getRoutingUrlAndVerifyHTTPResponse)

	// steps for verifying config file content
	s.Step(`^(JSON|YAML) config file "(.*)" (contains|does not contain) key "(.*)" with value matching "(.*)"$`,
		configFileContainsKeyMatchingValue)
	s.Step(`^(JSON|YAML) config file "(.*)" (has|does not have) key "(.*)"$`,
		configFileContainsKey)

	s.Step(`^(stdout|stderr) is (JSON|YAML) which (contains|does not contain) key "(.*)" with value matching "(.*)"$`,
		stdoutContainsKeyMatchingValue)
	s.Step(`^(stdout|stderr) is (JSON|YAML) which (has|does not have) key "(.*)"$`,
		stdoutContainsKey)

	// iso dependent steps
	s.Step(`^printing Docker daemon configuration to stdout$`,
		catDockerConfigFile)

	// steps for download of minishift-addons repository
	s.Step(`^file from "(.*)" is downloaded into location "(.*)"$`,
		downloadFileIntoLocation)

	// executing in terminal
	s.Step(`^user starts terminal instance$`,
		util.StartTerminal)
	s.Step(`^user closes terminal instance$`,
		util.CloseTerminal)
	s.Step(`^executing command "(.*)"$`,
		util.ExecuteInShell)

	s.BeforeSuite(func() {
		testDir = setUp()
		util.StartLog(testDir)
		fmt.Println("Running Integration test in:", testDir)
		fmt.Println("Using binary:", minishiftBinary)
	})

	s.AfterSuite(func() {
		util.LogMessage("info", "----- Cleaning Up -----")
		minishift.runner.EnsureDeleted()
		util.CloseLog()
	})

	s.BeforeFeature(func(this *gherkin.Feature) {
		util.LogMessage("info", "----- Preparing for feature -----")
		if runner.IsCDK() {
			runner.CDKSetup()
		} else {
			runner.RunCommand("addons list")
		}

		util.LogMessage("info", fmt.Sprintf("----- Feature: %s -----", this.Name))
	})

	s.AfterFeature(func(this *gherkin.Feature) {
		util.LogMessage("info", "----- Cleaning after feature -----")
		cleanTestDirConfiguration()
	})

	s.BeforeScenario(func(this interface{}) {
		switch this.(type) {
		case *gherkin.Scenario:
			scenario := *this.(*gherkin.Scenario)
			util.LogMessage("info", fmt.Sprintf("----- Scenario: %s -----", scenario.ScenarioDefinition.Name))
		case *gherkin.ScenarioOutline:
			scenario := *this.(*gherkin.ScenarioOutline)
			util.LogMessage("info", fmt.Sprintf("----- Scenario Outline: %s -----", scenario.ScenarioDefinition.Name))
		}
	})

	s.AfterScenario(func(interface{}, error) {
		testProxy.ResetLog(false)
	})

}

func setUp() string {
	if testDir == "" {
		testDir, _ = ioutil.TempDir("", "minishift-integration-test-")
	} else {
		ensureTestDirEmpty()
	}

	os.Setenv(constants.MiniShiftHomeEnv, testDir)
	return testDir
}

func ensureTestDirEmpty() {
	files, err := ioutil.ReadDir(testDir)
	if err != nil {
		fmt.Println(fmt.Sprintf("Unable to setup integration test directory: %v", err))
		os.Exit(1)
	}

	for _, file := range files {
		fullPath := filepath.Join(testDir, file.Name())
		if filepath.Base(file.Name()) == "cache" {
			fmt.Println(fmt.Sprintf("Keeping Minishift cache directory '%s' for test run.", fullPath))
			continue
		}
		os.RemoveAll(fullPath)
	}
}

func cleanTestDirConfiguration() {
	var foldersToClean []string
	foldersToClean = append(foldersToClean, filepath.Join(testDir, "addons"))
	foldersToClean = append(foldersToClean, filepath.Join(testDir, "config"))

	for index := range foldersToClean {
		err := os.RemoveAll(foldersToClean[index])
		if err != nil {
			fmt.Println(fmt.Sprintf("Unable to remove folder %v: %v", foldersToClean[index], err))
			os.Exit(1)
		}
	}
}

//  To get values of nested keys, use following dot formating in Scenarios: key.nestedKey
//  If an array is expected, then expect: "[value1 value2 value3]"
//  If empty string, non existing value are expected, then expect "<nil>"
func getConfigKeyValue(configData []byte, format string, keyPath string) (string, error) {
	var err error
	var keyValue string
	var values map[string]interface{}

	if format == "JSON" {
		err = json.Unmarshal(configData, &values)
		if err != nil {
			return "", fmt.Errorf("Error unmarshaling JSON: %s", err)
		}
	} else if format == "YAML" {
		err = yaml.Unmarshal(configData, &values)
		if err != nil {
			return "", fmt.Errorf("Error unmarshaling YAML: %s", err)
		}
	}

	keyPathArray := strings.Split(keyPath, ".")
	for _, element := range keyPathArray {
		switch value := values[element].(type) {
		case map[string]interface{}:
			values = value
		case map[interface{}]interface{}:
			retypedValue := make(map[string]interface{})
			for x := range value {
				retypedValue[x.(string)] = value[x]
			}
			values = retypedValue
		case []interface{}, nil, string, int, float64, bool:
			keyValue = fmt.Sprintf("%v", value)
		default:
			return "", errors.New("Unexpected type in config file, type not supported.")
		}
	}
	return keyValue, nil
}

func getFileContent(path string) ([]byte, error) {
	data, err := ioutil.ReadFile(testDir + "/" + path)
	if err != nil {
		return nil, fmt.Errorf("Cannot read file: %v", err)
	}

	return data, err
}

func configFileContainsKeyMatchingValue(format string, configPath string, condition string, keyPath string, expectedValue string) error {
	config, err := getFileContent(configPath)
	if err != nil {
		return err
	}

	keyValue, err := getConfigKeyValue(config, format, keyPath)
	if err != nil {
		return err
	}

	matches, err := performRegexMatch(expectedValue, keyValue)
	if err != nil {
		return err
	} else if (condition == "contains") && !matches {
		return fmt.Errorf("For key '%s' config contains unexpected value '%s'", keyPath, keyValue)
	} else if (condition == "does not contain") && matches {
		return fmt.Errorf("For key '%s' config contains value '%s', which it should not contain", keyPath, keyValue)
	}

	return nil
}

func configFileContainsKey(format string, configPath string, condition string, keyPath string) error {
	config, err := getFileContent(configPath)
	if err != nil {
		return err
	}

	keyValue, err := getConfigKeyValue(config, format, keyPath)
	if err != nil {
		return err
	}

	if (condition == "has") && (keyValue == "<nil>") {
		return fmt.Errorf("Config does not contain any value for key %s", keyPath)
	} else if (condition == "does not have") && (keyValue != "<nil>") {
		return fmt.Errorf("Config contains key %s with assigned value: %s", keyPath, keyValue)
	}

	return nil
}

func stdoutContainsKeyMatchingValue(commandField string, format string, condition string, keyPath string, expectedValue string) error {
	config := []byte(selectFieldFromLastOutput(commandField))

	keyValue, err := getConfigKeyValue(config, format, keyPath)
	if err != nil {
		return err
	}

	matches, err := performRegexMatch(expectedValue, keyValue)
	if err != nil {
		return err
	} else if (condition == "contains") && !matches {
		return fmt.Errorf("For key '%s' %s contains unexpected value '%s'", keyPath, commandField, keyValue)
	} else if (condition == "does not contain") && matches {
		return fmt.Errorf("For key '%s' %s contains value '%s', which it should not contain", keyPath, commandField, keyValue)
	}

	return nil
}

func stdoutContainsKey(commandField string, format string, condition string, keyPath string) error {
	config := []byte(selectFieldFromLastOutput(commandField))

	keyValue, err := getConfigKeyValue(config, format, keyPath)
	if err != nil {
		return err
	}

	if (condition == "has") && (keyValue == "<nil>") {
		return fmt.Errorf("%s does not contain any value for key %s", commandField, keyPath)
	} else if (condition == "does not have") && (keyValue != "<nil>") {
		return fmt.Errorf("%s contains key %s with assigned value: %s", commandField, keyPath, keyValue)
	}

	return nil
}

func compareExpectedWithActualContains(expected string, actual string) error {
	if !strings.Contains(actual, expected) {
		return fmt.Errorf("Output did not match. Expected: '%s', Actual: '%s'", expected, actual)
	}

	return nil
}

func compareExpectedWithActualNotContains(notexpected string, actual string) error {
	if strings.Contains(actual, notexpected) {
		return fmt.Errorf("Output did match. Not expected: '%s', Actual: '%s'", notexpected, actual)
	}

	return nil
}

func compareExpectedWithActualEquals(expected string, actual string) error {
	if actual != expected {
		return fmt.Errorf("Output did not match. Expected: '%s', Actual: '%s'", expected, actual)
	}

	return nil
}

func compareExpectedWithActualNotEquals(notexpected string, actual string) error {
	if actual == notexpected {
		return fmt.Errorf("Output did match. Not expected: '%s', Actual: '%s'", notexpected, actual)
	}

	return nil
}

func performRegexMatch(regex string, input string) (bool, error) {
	compRegex, err := regexp.Compile(regex)
	if err != nil {
		return false, fmt.Errorf("Expected value must be a valid regular expression statement: ", err)
	}

	return compRegex.MatchString(input), nil
}

func compareExpectedWithActualMatchesRegex(expected string, actual string) error {
	matches, err := performRegexMatch(expected, actual)
	if err != nil {
		return err
	} else if !matches {
		return fmt.Errorf("Output did not match. Expected: '%s', Actual: '%s'", expected, actual)
	}

	return nil
}

func compareExpectedWithActualNotMatchesRegex(notexpected string, actual string) error {
	matches, err := performRegexMatch(notexpected, actual)
	if err != nil {
		return err
	} else if matches {
		return fmt.Errorf("Output did match. Not expected: '%s', Actual: '%s'", notexpected, actual)
	}

	return nil
}

func getLastCommandOutput() CommandOutput {
	return commandOutputs[len(commandOutputs)-1]
}

func selectFieldFromLastOutput(commandField string) string {
	lastCommandOutput := getLastCommandOutput()
	outputField := ""
	switch commandField {
	case "stdout":
		outputField = lastCommandOutput.StdOut
	case "stderr":
		outputField = lastCommandOutput.StdErr
	case "exitcode":
		outputField = strconv.Itoa(lastCommandOutput.ExitCode)
	}
	return outputField
}

func shouldBeInValidFormat(commandField string, format string) error {
	result := selectFieldFromLastOutput(commandField)
	result = strings.TrimRight(result, "\n")
	var err error
	switch format {
	case "URL":
		_, err = validateURL(result)
	case "IP":
		_, err = validateIP(result)
	case "IP with port number":
		_, err = validateIPWithPort(result)
	case "YAML":
		_, err = validateYAML(result)
	default:
		return fmt.Errorf("Format %s not implemented.", format)
	}

	return err
}

func validateIP(inputString string) (bool, error) {
	if net.ParseIP(inputString) == nil {
		return false, fmt.Errorf("IP address '%s' is not a valid IP address", inputString)
	}

	return true, nil
}

func validateURL(inputString string) (bool, error) {
	_, err := url.ParseRequestURI(inputString)
	if err != nil {
		return false, fmt.Errorf("URL '%s' is not an URL in valid format. Parsing error: %v", inputString, err)
	}

	return true, nil
}

func validateIPWithPort(inputString string) (bool, error) {
	split := strings.Split(inputString, ":")
	if len(split) != 2 {
		return false, fmt.Errorf("String '%s' does not contain one ':' separator", inputString)
	}
	if _, err := strconv.Atoi(split[1]); err != nil {
		return false, fmt.Errorf("Port must be an integer, in '%s' the port '%s' is not an integer. Conversion error: %v", inputString, split[1], err)
	}
	if net.ParseIP(split[0]) == nil {
		return false, fmt.Errorf("In '%s' the IP part '%s' is not a valid IP address", inputString, split[0])
	}

	return true, nil
}

func validateYAML(inputString string) (bool, error) {
	m := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(inputString), &m)
	if err != nil {
		return false, fmt.Errorf("Error unmarshaling YAML: %s. YAML='%s'", err, inputString)
	}

	return true, nil
}

func commandReturnEquals(commandField string, command string, condition string, expected string) error {
	minishift.executingMinishiftCommand(command)
	if condition == "is equal" {
		return compareExpectedWithActualEquals(expected+"\n", selectFieldFromLastOutput(commandField))
	} else {
		return compareExpectedWithActualNotEquals(expected+"\n", selectFieldFromLastOutput(commandField))
	}
}

func commandReturnContains(commandField string, command string, condition string, expected string) error {
	minishift.executingMinishiftCommand(command)
	if condition == "contains" {
		return compareExpectedWithActualContains(expected, selectFieldFromLastOutput(commandField))
	} else {
		return compareExpectedWithActualNotContains(expected, selectFieldFromLastOutput(commandField))
	}
}

func commandReturnShouldContain(commandField string, expected string) error {
	return compareExpectedWithActualContains(expected, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldNotContain(commandField string, notexpected string) error {
	return compareExpectedWithActualNotContains(notexpected, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldContainContent(commandField string, expected *gherkin.DocString) error {
	return compareExpectedWithActualContains(expected.Content, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldNotContainContent(commandField string, notexpected *gherkin.DocString) error {
	return compareExpectedWithActualNotContains(notexpected.Content, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldEqual(commandField string, expected string) error {
	return compareExpectedWithActualEquals(expected, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldEqualContent(commandField string, expected *gherkin.DocString) error {
	return compareExpectedWithActualEquals(expected.Content, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldBeEmpty(commandField string) error {
	return compareExpectedWithActualEquals("", selectFieldFromLastOutput(commandField))
}

func commandReturnShouldNotBeEmpty(commandField string) error {
	return compareExpectedWithActualNotEquals("", selectFieldFromLastOutput(commandField))
}

func variableShouldNotBeEmpty(variableName string) error {
	return compareExpectedWithActualNotEquals("", minishift.GetVariableByName(variableName).Value)
}

func commandReturnShouldMatchRegex(commandField string, expected string) error {
	return compareExpectedWithActualMatchesRegex(expected, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldNotMatchRegex(commandField string, notexpected string) error {
	return compareExpectedWithActualNotMatchesRegex(notexpected, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldMatchRegexContent(commandField string, expected *gherkin.DocString) error {
	return compareExpectedWithActualMatchesRegex(expected.Content, selectFieldFromLastOutput(commandField))
}

func commandReturnShouldNotMatchRegexContent(commandField string, notexpected *gherkin.DocString) error {
	return compareExpectedWithActualNotMatchesRegex(notexpected.Content, selectFieldFromLastOutput(commandField))
}

type commandRunner func(string) error

func executingOcCommandSucceedsOrFails(command string, expectedResult string) error {
	return succeedsOrFails(minishift.executingOcCommand, command, expectedResult)
}

func executingMinishiftCommandSucceedsOrFails(command string, expectedResult string) error {
	return succeedsOrFails(minishift.executingMinishiftCommand, command, expectedResult)
}

func succeedsOrFails(execute commandRunner, command string, expectedResult string) error {
	err := execute(command)
	if err != nil {
		return err
	}

	lastCommandOutput := getLastCommandOutput()
	commandFailed := (lastCommandOutput.ExitCode != 0 ||
		len(lastCommandOutput.StdErr) != 0)

	if expectedResult == "succeeds" && commandFailed == true {
		return fmt.Errorf("Command '%s' did not execute successfully. cmdExit: %d, cmdErr: %s",
			lastCommandOutput.Command,
			lastCommandOutput.ExitCode,
			lastCommandOutput.StdErr)
	}
	if expectedResult == "fails" && commandFailed == false {
		return fmt.Errorf("Command executed successfully, however was expected to fail. cmdExit: %d, cmdErr: %s",
			lastCommandOutput.ExitCode,
			lastCommandOutput.StdErr)
	}

	return nil
}

func verifyHTTPResponse(partOfResponse string, url string, urlSuffix string, assertion string, expected string) error {
	switch url {
	case "OpenShift":
		url = minishift.getOpenShiftUrl() + urlSuffix
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("Server returned error on url: %s", url)
	}
	defer response.Body.Close()
	var result string
	switch partOfResponse {
	case "body":
		html, _ := ioutil.ReadAll(response.Body)
		result = string(html[:])
	case "status code":
		result = fmt.Sprintf("%d", response.StatusCode)
	default:
		return fmt.Errorf("%s not implemented", partOfResponse)
	}

	switch assertion {
	case "contains":
		if !strings.Contains(result, expected) {
			return fmt.Errorf("%s of reponse from %s does not contain expected string. Expected: %s, Actual: %s", partOfResponse, url, expected, result)
		}
	case "is equal to":
		if result != expected {
			return fmt.Errorf("%s of response from %s is not equal to expected string. Expected: %s, Actual: %s", partOfResponse, url, expected, result)
		}
	default:
		return fmt.Errorf("Assertion type: %s is not implemented", assertion)
	}
	return nil
}

func getRoutingUrlAndVerifyHTTPResponse(partOfResponse string, urlRoot string, serviceName string, nameSpace string, assertion string, expected string) error {
	url := minishift.getRoute(serviceName, nameSpace)
	if urlRoot == "/" {
		return verifyHTTPResponse(partOfResponse, url, "", assertion, expected)
	} else if strings.HasPrefix(urlRoot, "/") {
		url := url + urlRoot
		return verifyHTTPResponse(partOfResponse, url, "", assertion, expected)
	} else {
		return fmt.Errorf("Wrong input format : %s. Input must start with /", urlRoot)
	}
	return nil
}

func proxyLogShouldContain(expected string) error {
	return compareExpectedWithActualContains(expected, testProxy.GetLog())
}

func proxyLogShouldContainContent(expected *gherkin.DocString) error {
	return compareExpectedWithActualContains(expected.Content, testProxy.GetLog())
}

func catDockerConfigFile() error {
	var err error
	if isoName == "b2d" {
		err = executingMinishiftCommandSucceedsOrFails("ssh -- cat /var/lib/boot2docker/profile", "succeeds")

	} else if isoName == "centos" || isoName == "rhel" {
		err = executingMinishiftCommandSucceedsOrFails("ssh -- cat /etc/systemd/system/docker.service.d/10-machine.conf", "succeeds")

	} else {
		return errors.New("ISO name not supported.")
	}

	return err
}

func downloadFileIntoLocation(downloadURL string, destinationFolder string) error {
	destinationFolder = filepath.Join(testDir, destinationFolder)
	err := os.MkdirAll(destinationFolder, os.ModePerm)
	if err != nil {
		return err
	}

	slice := strings.Split(downloadURL, "/")
	fileName := slice[len(slice)-1]
	filePath := filepath.Join(destinationFolder, fileName)
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
