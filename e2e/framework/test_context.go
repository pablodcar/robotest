package framework

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/gravitational/configure"
	"github.com/gravitational/robotest/e2e/framework/defaults"
	"github.com/gravitational/robotest/infra"
	"github.com/gravitational/robotest/infra/terraform"
	"github.com/gravitational/robotest/infra/vagrant"
	"github.com/gravitational/robotest/lib/debug"
	"github.com/gravitational/robotest/lib/loc"
	"github.com/gravitational/trace"

	"github.com/go-yaml/yaml"
	"github.com/kr/pretty"
	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	log "github.com/sirupsen/logrus"
)

// ConfigureFlags registers common command line flags, parses the command line
// and interprets the configuration
func ConfigureFlags() {
	registerCommonFlags()

	flag.Parse()

	initLogger(debugFlag)

	if debugFlag {
		debug.StartProfiling(fmt.Sprintf("localhost:%v", debugPort))
	}

	err := initTestContext(configFile)
	if err != nil {
		Failf("failed to read configuration from %q: %v", configFile, trace.DebugReport(err))
	}

	err = initTestState(stateConfigFile)
	if err != nil {
		Failf("failed to read state configuration from %q: %v", stateConfigFile, trace.DebugReport(err))
	}

	if testState != nil {
		// testState -> TestContext
		TestContext.Provisioner = testState.Provisioner
		TestContext.StateDir = testState.StateDir
		if testState.EntryURL != "" {
			TestContext.OpsCenterURL = testState.EntryURL
		}
		if testState.Login != nil {
			TestContext.Login = *testState.Login
		}
		if testState.ServiceLogin != nil {
			TestContext.ServiceLogin = *testState.ServiceLogin
		}
		if testState.ProvisionerState != nil {
			TestContext.Wizard = testState.ProvisionerState.InstallerAddr != ""
		}
		if testState.Application != nil {
			TestContext.Application.Locator = testState.Application
		}
		if testState.Bandwagon != nil {
			TestContext.Bandwagon = *testState.Bandwagon
		}
	}

	if mode == wizardMode || TestContext.Wizard {
		TestContext.Wizard = true
	}

	if mode == provisionMode {
		// Skip tests for this operation
		config.GinkgoConfig.SkipString = ".*"
	}

	if provisionerName != "" {
		TestContext.Provisioner = provisionerType(provisionerName)
	}

	if teardownFlag {
		TestContext.Teardown = teardownFlag
	}

	if dumpFlag {
		TestContext.DumpCore = dumpFlag
	}

	if stateDir != "" {
		TestContext.StateDir = stateDir
	}

	if TestContext.Teardown || TestContext.DumpCore {
		// Skip tests for this operation
		config.GinkgoConfig.SkipString = ".*"
	}

	outputSensitiveConfig(*TestContext)
	if testState != nil {
		outputSensitiveState(*testState)
		if testState.ProvisionerState != nil {
			log.Debugf("[PROVISIONER STATE]: %#v", testState)
		}
	}
}

func (r *TestContextType) Validate() error {
	var errors []error
	if mode == wizardMode && TestContext.Onprem.InstallerURL == "" {
		errors = append(errors, trace.BadParameter("installer URL is required in wizard mode"))
	}
	var command bool = teardownFlag || dumpFlag || mode == wizardMode
	if !command && TestContext.Login.IsEmpty() {
		log.Warningf("Ops Center login not configured - Ops Center access will likely not be available")
	}
	if TestContext.ServiceLogin.IsEmpty() {
		log.Warningf("service login not configured - reports will likely not be collected")
	}
	if TestContext.Provisioner != "" && TestContext.Onprem.IsEmpty() {
		errors = append(errors, trace.BadParameter("Onprem configuration is required for provisioner %q",
			TestContext.Provisioner))
	}
	// Do not mandate AWS.AccessKey/AWS.SecretKey for terraform as scripts can be written to consume
	// credentials not only from environment
	return trace.NewAggregate(errors...)
}

func Failf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Error(msg)
	ginkgo.Fail(nowStamp()+": "+msg, 1)
}

// TestContext defines the global test configuration for the test run
var TestContext = &TestContextType{
	Bandwagon: BandwagonConfig{
		Organization: defaults.BandwagonOrganization,
		Username:     defaults.BandwagonUsername,
		Email:        defaults.BandwagonEmail,
		Password:     defaults.BandwagonPassword,
	},
}

// testState defines an optional state configuration that allows the test runner
// to use state from previous runs
var testState *TestState

// TestContextType defines the configuration context of a single test run
type TestContextType struct {
	// Wizard specifies whether wizard was used to bootstrap cluster
	Wizard bool `json:"-" yaml:"-"`
	// Provisioner defines the type of provisioner to use
	Provisioner provisionerType `json:"provisioner" yaml:"provisioner" `
	// CloudProvider defines cloud to deploy
	CloudProvider string `json:"cloud_provider" yaml:"cloud_provider" validate:"omitempty,eq=aws|eq=azure"`
	// DumpCore specifies a command to collect all installation/operation logs
	DumpCore bool `json:"-" yaml:"-"`
	// StateDir specifies the location for test-specific temporary data
	StateDir string `json:"state_dir" yaml:"state_dir" `
	// Teardown specifies the command to destroy the infrastructure
	Teardown bool `json:"-" yaml:"-"`
	// ForceRemoteAccess explicitly enables the remote access for the installed site.
	// If unspecified (or false), remote access is configured automatically:
	//  - if installing into existing Ops Center, remote access is enabled
	//  - in wizard mode remote access is disabled
	//
	// TODO: automatically determine when to enable remote access
	ForceRemoteAccess bool `json:"remote_access,omitempty" yaml:"remote_access,omitempty" `
	// ForceLocalEndpoint specifies whether to use the local application endpoint
	// instead of Ops Center to control the installed site
	//
	// TODO: automatically determine when to use local endpoint
	ForceLocalEndpoint bool `json:"local_endpoint,omitempty" yaml:"local_endpoint,omitempty" `
	// ReportDir defines location to store the results of the test
	ReportDir string `json:"report_dir" yaml:"report_dir" `
	// ClusterName defines the name to use for domain name or state directory
	ClusterName string `json:"cluster_name" yaml:"cluster_name" `
	// License specifies the application license
	License string `json:"license" yaml:"license" `
	// OpsCenterURL specifies the Ops Center to use for tests.
	// OpsCenterURL is mandatory when running tests on an existing Ops Center.
	// In wizard mode, this is automatically populated by the wizard (incl. Application, see below)
	OpsCenterURL string `json:"ops_url" yaml:"ops_url" `
	// Application defines the application package to test.
	// In wizard mode, this is automatically set by the wizard
	Application LocatorRef `json:"application" yaml:"application"`
	// Login defines the login details to access existing Ops Center.
	// Mandatory only in non-wizard mode
	Login Login `json:"login" yaml:"login"`
	// ServiceLogin defines the login parameters for service access to the Ops Center
	ServiceLogin ServiceLogin `json:"service_login" yaml:"service_login"`
	// FlavorLabel specifies the installation flavor label to use for the test.
	// This is application-specific, e.g. `3 nodes` or `medium`
	FlavorLabel string `json:"flavor_label" yaml:"flavor_label" `

	// AWS defines the AWS-specific test configuration
	AWS *infra.AWSConfig `json:"aws" yaml:"aws"`
	// Azure defines Azure cloud specific parameters
	Azure *infra.AzureConfig `yaml:"azure"`

	// Onprem defines the test configuration for bare metal tests
	Onprem OnpremConfig `json:"onprem" yaml:"onprem"`
	// Bandwagon defines the test configuration for post-install setup in bandwagon
	Bandwagon BandwagonConfig `json:"bandwagon" yaml:"bandwagon"`
	// WebDriverURL specifies optional WebDriver URL to use
	WebDriverURL string `json:"web_driver_url,omitempty" yaml:"web_driver_url,omitempty" `
	// Extensions groups arbitrary test step configuration
	Extensions Extensions `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

type BandwagonConfig struct {
	Organization string `json:"organization" yaml:"organization" `
	Username     string `json:"username" yaml:"username" `
	Password     string `json:"password" yaml:"password" `
	Email        string `json:"email" yaml:"email" `
	RemoteAccess bool
}

// Login defines Ops Center authentication parameters
type Login struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	// AuthProvider specifies the authentication provider to use for login.
	// Available providers are `email` and `gogole`
	AuthProvider string `json:"auth_provider,omitempty" yaml:"auth_provider,omitempty"`
}

func (r Login) IsEmpty() bool {
	return r.Username == "" && r.Password == ""
}

// ServiceLogin defines authentication options for Ops Center service access
type ServiceLogin struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

func (r ServiceLogin) IsEmpty() bool {
	return r.Username == "" && r.Password == ""
}

// OnpremConfig defines the test configuration for bare metal tests
type OnpremConfig struct {
	// Onprem
	// NumNodes defines the total cluster capacity.
	// This is a total number of nodes to provision
	NumNodes int `json:"nodes" yaml:"nodes"`
	// InstallerURL defines the location of the installer tarball.
	// Depending on the provisioner - this can be either a URL or local path
	InstallerURL string `json:"installer_url" yaml:"installer_url"`
	// ScriptPath defines the path to the provisioner script.
	// TODO: if unspecified, scripts in assets/<provisioner> are used
	ScriptPath string `json:"script_path" yaml:"script_path"`
	// ExpandProfile specifies an optional name of the server profile for On-Premise expand operation.
	// If the profile is unspecified, the test will use the first available.
	ExpandProfile string `json:"expand_profile" yaml:"expand_profile"`
	// OS defines OS flavor, ubuntu | redhat | centos | debian
	OS string `json:"os" yaml:"os" validate:"required,eq=ubuntu|eq=redhat|eq=centos|eq=debian"`
	// DockerDevice block device for docker data - set to /dev/xvdb
	DockerDevice string `json:"docker_device" yaml:"docker_device" validate:"required"`
}

func (r OnpremConfig) IsEmpty() bool {
	return r.NumNodes == 0 && r.ScriptPath == ""
}

// LocatorRef defines a reference to a package locator.
// It is necessary to keep application package optional
// in the configuration while being able to consume value
// from environment
type LocatorRef struct {
	*loc.Locator
}

// SetEnv implements configure.EnvSetter
func (r *LocatorRef) SetEnv(value string) error {
	var loc loc.Locator
	err := loc.UnmarshalText([]byte(value))
	if err != nil {
		return err
	}
	r.Locator = &loc
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (r *LocatorRef) UnmarshalText(p []byte) error {
	loc := &loc.Locator{}
	err := loc.UnmarshalText(p)
	if err != nil {
		return err
	}
	r.Locator = loc
	return nil
}

// UnmarshalText implements encoding.TextMarshaler
func (r LocatorRef) MarshalText() ([]byte, error) {
	return r.Locator.MarshalText()
}

func registerCommonFlags() {
	// Turn on verbose by default to get spec names
	config.DefaultReporterConfig.Verbose = true
	// Turn on EmitSpecProgress to get spec progress (especially on interrupt)
	config.GinkgoConfig.EmitSpecProgress = true

	flag.StringVar(&configFile, "config", "config.yaml", "Configuration file to use")
	flag.StringVar(&stateDir, "state-dir", "", "Directory to store state in")
	flag.StringVar(&stateConfigFile, "state-file", "config.yaml.state", "State configuration file to use")
	flag.BoolVar(&debugFlag, "debug", false, "Verbose mode")
	flag.IntVar(&debugPort, "debug-port", 6060, "Profiling port")
	flag.Var(&mode, "mode", "Run robotest in specific mode. Supported modes: [`wizard`,`provision`]")
	flag.BoolVar(&teardownFlag, "destroy", false, "Destroy infrastructure after all tests")
	flag.BoolVar(&outputFlag, "output", false, "Display current state only")
	flag.BoolVar(&dumpFlag, "report", false, "Collect installation and operation logs into the report directory")
	flag.StringVar(&provisionerName, "provisioner", "", "Provision nodes using this provisioner")
}

func initTestContext(confFile string) error {
	err := newContextConfig(confFile)
	if err != nil {
		return trace.Wrap(err, "failed to read configuration from %q", confFile)
	}

	err = configure.ParseEnv(TestContext)
	if err != nil {
		return trace.Wrap(err, "failed to update configuration from environment")
	}

	err = TestContext.Validate()
	if err != nil {
		return trace.Wrap(err, "config validation failed")
	}

	return nil
}

func newContextConfig(configFile string) error {
	confFile, err := os.Open(configFile)
	if err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err)
	}
	if confFile == nil {
		// No configuration file - consume configuration from environment
		return nil
	}

	defer confFile.Close()

	configBytes, err := ioutil.ReadAll(confFile)
	if err != nil {
		return trace.Wrap(err)
	}

	err = yaml.Unmarshal(configBytes, &TestContext)
	if err != nil {
		return trace.Wrap(err, "Error parsing config file")
	}

	return nil
}

func initTestState(configFile string) error {
	confFile, err := os.Open(configFile)
	if err != nil && !os.IsNotExist(err) {
		return trace.ConvertSystemError(err)
	}
	if err != nil {
		// No test state configuration
		return nil
	}
	defer confFile.Close()

	var state TestState
	d := json.NewDecoder(confFile)
	err = d.Decode(&state)
	if err != nil {
		return trace.Wrap(err)
	}

	err = state.Validate()
	if err != nil {
		return trace.Wrap(err, "failed to validate state configuration")
	}

	testState = &state
	return nil
}

func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}

func initLogger(debug bool) {
	level := log.InfoLevel
	if debug {
		level = log.DebugLevel
	}
	log.StandardLogger().Hooks = make(log.LevelHooks)
	log.SetFormatter(&trace.TextFormatter{TextFormatter: log.TextFormatter{FullTimestamp: true}})
	log.SetOutput(os.Stderr)
	log.SetLevel(level)
}

func makeTerraformConfig(infraConfig infra.Config) (config *terraform.Config, err error) {
	if TestContext.CloudProvider == "" {
		return nil, trace.Errorf("cloud_provider parameter is required for Terraform")
	}

	config = &terraform.Config{
		Config:        infraConfig,
		ScriptPath:    TestContext.Onprem.ScriptPath,
		InstallerURL:  TestContext.Onprem.InstallerURL,
		NumNodes:      TestContext.Onprem.NumNodes,
		OS:            TestContext.Onprem.OS,
		CloudProvider: TestContext.CloudProvider,
		AWS:           TestContext.AWS,
		Azure:         TestContext.Azure,
	}

	err = config.Validate()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return config, nil
}

func provisionerFromConfig(infraConfig infra.Config, stateDir string, provisionerName provisionerType) (provisioner infra.Provisioner, err error) {
	switch provisionerName {
	case provisionerTerraform:
		config, err := makeTerraformConfig(infraConfig)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		provisioner, err = terraform.New(stateDir, *config)
	case provisionerVagrant:
		config := vagrant.Config{
			Config:       infraConfig,
			ScriptPath:   TestContext.Onprem.ScriptPath,
			InstallerURL: TestContext.Onprem.InstallerURL,
			NumNodes:     TestContext.Onprem.NumNodes,
		}
		err := config.Validate()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		provisioner, err = vagrant.New(stateDir, config)
	default:
		// no provisioner when the cluster has already been provisioned
		// or automatic provisioning is used
		if provisionerName != "" {
			return nil, trace.BadParameter("unknown provisioner %q", provisionerName)
		}
	}
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return provisioner, nil
}

func provisionerFromState(infraConfig infra.Config, testState TestState) (provisioner infra.Provisioner, err error) {
	err = testState.Validate()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	numNodes := len(testState.ProvisionerState.Nodes)
	if TestContext.Onprem.NumNodes > 0 {
		// Always override from configuration if available
		numNodes = TestContext.Onprem.NumNodes
	}
	switch testState.Provisioner {
	case provisionerTerraform:
		config, err := makeTerraformConfig(infraConfig)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		provisioner, err = terraform.NewFromState(*config, *testState.ProvisionerState)
	case provisionerVagrant:
		config := vagrant.Config{
			Config:       infraConfig,
			ScriptPath:   TestContext.Onprem.ScriptPath,
			InstallerURL: TestContext.Onprem.InstallerURL,
			NumNodes:     numNodes,
		}
		err := config.Validate()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		provisioner, err = vagrant.NewFromState(config, *testState.ProvisionerState)
	default:
		// no provisioner when the cluster has already been provisioned
		// or automatic provisioning is used
		if testState.Provisioner != "" {
			return nil, trace.BadParameter("unknown provisioner %q", testState.Provisioner)
		}
	}
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return provisioner, nil
}

func outputSensitiveConfig(testConfig TestContextType) {
	testConfig.AWS = nil
	testConfig.Azure = nil
	testConfig.Login.Password = mask
	testConfig.ServiceLogin.Password = mask
	var buf bytes.Buffer
	pretty.Fprintf(&buf, "[CONFIG] %# v", testConfig)
	log.Debug(buf.String())
}

func outputSensitiveState(testState TestState) {
	if testState.Login != nil {
		login := &Login{}
		*login = *testState.Login
		login.Password = mask
		testState.Login = login
	}
	if testState.ServiceLogin != nil {
		login := &ServiceLogin{}
		*login = *testState.ServiceLogin
		login.Password = mask
		testState.ServiceLogin = login
	}
	var buf bytes.Buffer
	pretty.Fprintf(&buf, "[STATE]: %# v", testState)
	log.Debug(buf.String())
}

const mask = "****"

type provisionerType string

const (
	provisionerTerraform provisionerType = "terraform"
	provisionerVagrant   provisionerType = "vagrant"
)

func (r *modeType) String() string {
	return string(*r)
}

func (r *modeType) Set(value string) error {
	*r = modeType(value)
	if *r == "" {
		*r = wizardMode
	}
	return nil
}

type modeType string

const (
	wizardMode    modeType = "wizard"
	provisionMode modeType = "provision"
)

// configFile defines the configuration file to use for the tests
var configFile string

// stateConfigFile defines the state configuration file to use for the tests
var stateConfigFile string

// debugFlag defines whether to run in verbose mode
var debugFlag bool

// debugPort defines the port for profiling endpoint
var debugPort int

// mode defines the mode for tests
var mode modeType

// provisionerName defines the provisioner to use to provision nodes in the test cluster
var provisionerName string

// teardownFlag defines if the cluster should be destroyed
var teardownFlag bool

// outputFlag defines if only current state should be output
var outputFlag bool

// dumpFlag defines whether to collect installation and operation logs
var dumpFlag bool

// stateDir defines a user specified directory to store state in
var stateDir string
