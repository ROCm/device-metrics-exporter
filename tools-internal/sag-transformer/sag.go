/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "sigs.k8s.io/yaml"
)

/*---SAG JSON related definitions---*/
type ServiceAction struct {
	Title string   `json:"title"`
	Steps []string `json:"steps"`
}

type AFID struct {
	ErrorCategory    string   `json:"error_category"`
	ErrorType        string   `json:"error_type"`
	ErrorSeverity    string   `json:"error_severity"`
	Threshold        int      `json:"threshold"`
	Priority         int      `json:"priority"`
	Fru              string   `json:"fru"`
	Status           string   `json:"status"`
	Method           string   `json:"method"`
	SupportedSystems []string `json:"supported_systems"`
	ServiceActionNum int      `json:"service_action_num"`
	ServiceAction    string   `json:"service_action"`
}

type AFIDSAG struct {
	PID            string                   `json:"pid"`
	Variant        string                   `json:"variant"`
	Revision       string                   `json:"revision"`
	AFIDRevision   float64                  `json:"afid_revision"`
	SAGRevision    float64                  `json:"sag_revision"`
	RFRevision     float64                  `json:"rf_revision"`
	Notes          string                   `json:"notes"`
	Afid           map[string]AFID          `json:"afid"`
	ServiceActions map[string]ServiceAction `json:"service_actions"`
}

type AfidSaMap struct {
	Version string            `json:"version"`
	Mapping map[string]string `json:"afid_sa_mapping"`
}

type ValidationTestsProfile struct {
	Framework      string `json:"framework" yaml:"framework"`
	Recipe         string `json:"recipe" yaml:"recipe"`
	Iterations     int    `json:"iterations" yaml:"iterations"`
	StopOnFailure  bool   `json:"stopOnFailure" yaml:"stopOnFailure"`
	TimeoutSeconds int    `json:"timeoutSeconds" yaml:"timeoutSeconds"`
}

type RecoveryPolicyConfig struct {
	MaxAllowedRunsPerWindow int    `json:"maxAllowedRunsPerWindow" yaml:"maxAllowedRunsPerWindow"`
	WindowSize              string `json:"windowSize" yaml:"windowSize"`
}

type ConditionWorkflowMapping struct {
	Afid                     string                 `json:"afid" yaml:"afid"`
	NodeCondition            string                 `json:"nodeCondition" yaml:"nodeCondition"`
	WorkflowTemplate         string                 `json:"workflowTemplate" yaml:"workflowTemplate"`
	ValidationTests          ValidationTestsProfile `json:"validationTestsProfile" yaml:"validationTestsProfile"`
	PhysicalActionNeeded     string                 `json:"physicalActionNeeded" yaml:"physicalActionNeeded"`
	NotifyRemediationMessage string                 `json:"notifyRemediationMessage" yaml:"notifyRemediationMessage"`
	NotifyTestFailureMessage string                 `json:"notifyTestFailureMessage" yaml:"notifyTestFailureMessage"`
	RecoveryPolicy           RecoveryPolicyConfig   `json:"recoveryPolicy" yaml:"recoveryPolicy"`
	Threshold                int                    `json:"threshold" yaml:"threshold"`
}

/*---NPD custom plugin monitor config related definitions---*/
type Type string

const (
	Temp Type = "temporary"
	Perm Type = "permanent"
)

type CustomRule struct {
	Type          Type           `json:"type"`
	Condition     string         `json:"condition"`
	Reason        string         `json:"reason"`
	Path          string         `json:"path"`
	Args          []string       `json:"args"`
	TimeoutString *string        `json:"timeout"`
	Timeout       *time.Duration `json:"-"`
}

type pluginGlobalConfig struct {
	InvokeIntervalString                    *string        `json:"invoke_interval,omitempty"`
	TimeoutString                           *string        `json:"timeout,omitempty"`
	InvokeInterval                          *time.Duration `json:"-"`
	Timeout                                 *time.Duration `json:"-"`
	MaxOutputLength                         *int           `json:"max_output_length,omitempty"`
	Concurrency                             *int           `json:"concurrency,omitempty"`
	EnableMessageChangeBasedConditionUpdate *bool          `json:"enable_message_change_based_condition_update,omitempty"`
	SkipInitialStatus                       *bool          `json:"skip_initial_status,omitempty"`
}

type ConditionStatus string

const (
	True    ConditionStatus = "True"
	False   ConditionStatus = "False"
	Unknown ConditionStatus = "Unknown"
)

type Condition struct {
	Type       string          `json:"type"`
	Status     ConditionStatus `json:"status,omitempty"`
	Transition time.Time       `json:"-"`
	Reason     string          `json:"reason"`
	Message    string          `json:"message"`
}

type CustomPluginConfig struct {
	Plugin                 string             `json:"plugin,omitempty"`
	PluginGlobalConfig     pluginGlobalConfig `json:"pluginConfig,omitempty"`
	Source                 string             `json:"source"`
	DefaultConditions      []Condition        `json:"conditions"`
	Rules                  []*CustomRule      `json:"rules"`
	EnableMetricsReporting *bool              `json:"metricsReporting,omitempty"`
}

var (
	defaultInvokeInterval                    = 30 * time.Second
	defaultInvokeIntervalString              = defaultInvokeInterval.String()
	defaultGlobalTimeout                     = 10 * time.Second
	defaultGlobalTimeoutString               = defaultGlobalTimeout.String()
	defaultMaxOutputLength                   = 80
	defaultConcurrency                       = 3
	defaultMessageChangeBasedConditionUpdate = false
	defaultEnableMetricsReporting            = true
)

var agfhcTestProfileRe = regexp.MustCompile(`/opt/amd/agfhc/agfhc -([rt]) (\w+)`)
var agfhcRecipeRe = regexp.MustCompile(`/opt/amd/agfhc/agfhc -r`)
var failureNotificationRe = regexp.MustCompile(`AGFHC[\w\s]*fail[,\(\w\s]*SA([\d])`)
var nodeConditionRe = regexp.MustCompile(`\(\w*\)`)

func parseAfidJSON(path string) (*AFIDSAG, error) {
	var afidSag AFIDSAG
	jsondata, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("unable to read json")
		return nil, err
	}
	err = json.Unmarshal(jsondata, &afidSag)
	if err != nil {
		fmt.Printf("unable to unmarshal the json data. Error %v\n", err)
		return nil, err
	}
	return &afidSag, err
}

func (a *AFIDSAG) getValidationTestsProfile(id string) ValidationTestsProfile {
	afid := a.Afid[id]
	sa := a.ServiceActions[strconv.Itoa(afid.ServiceActionNum)]
	output := ValidationTestsProfile{
		Framework:      "AGFHC",
		Iterations:     1,
		StopOnFailure:  true,
		TimeoutSeconds: 4800,
	}
	output.Recipe = "all_lvl4"
	for _, step := range sa.Steps {
		matchedTestProfile := agfhcTestProfileRe.FindStringSubmatch(step)
		if len(matchedTestProfile) != 3 {
			continue
		}
		output.Recipe = matchedTestProfile[2]
		break
	}
	return output
}

func (a *AFIDSAG) getNotifyRemediationMessage(id, sac string) string {
	if id != "" {
		afid := a.Afid[id]
		sac = strconv.Itoa(afid.ServiceActionNum)
	}
	sa := a.ServiceActions[sac]
	found := false
	notifyMessage := ""
	for idx, step := range sa.Steps {
		if idx == 0 && step == "Take the node out of service." {
			continue
		}
		found = agfhcRecipeRe.MatchString(step)
		if found {
			break
		}
		notifyMessage = notifyMessage + step
	}
	return notifyMessage
}

func (a *AFIDSAG) getNotifyTestFailureMessage(id string) string {
	afid := a.Afid[id]
	sa := a.ServiceActions[strconv.Itoa(afid.ServiceActionNum)]
	for _, step := range sa.Steps {
		matches := failureNotificationRe.FindStringSubmatch(step)
		if len(matches) != 2 {
			continue
		}
		return a.getNotifyRemediationMessage("", matches[1])
	}
	return "Check test run logs for next steps."
}

func ToPascalCase(s string) string {
	var result []rune
	capitalizeNext := true

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if capitalizeNext {
				result = append(result, unicode.ToUpper(r))
				capitalizeNext = false
			} else {
				result = append(result, unicode.ToLower(r))
			}
		} else {
			capitalizeNext = true
		}
	}
	return string(result)
}

func sanitizeNodeCondition(nc string) string {
	outStr := nodeConditionRe.ReplaceAllString(nc, "")
	separatorFunc := func(r rune) bool {
		return r == ' ' || r == '-'
	}
	output := "AMDGPU"
	strs := strings.FieldsFunc(outStr, separatorFunc)

	for idx, str := range strs {
		tmp := ToPascalCase(str)
		if idx == 0 && tmp == "Gpu" {
			continue
		}
		output = output + tmp
	}
	return output
}

func getBaseNPDConfig() *CustomPluginConfig {
	op := &CustomPluginConfig{}
	op.Plugin = "custom"
	op.PluginGlobalConfig = pluginGlobalConfig{
		InvokeIntervalString:                    &defaultInvokeIntervalString,
		TimeoutString:                           &defaultGlobalTimeoutString,
		MaxOutputLength:                         &defaultMaxOutputLength,
		Concurrency:                             &defaultConcurrency,
		EnableMessageChangeBasedConditionUpdate: &defaultMessageChangeBasedConditionUpdate,
	}
	op.Source = "amdgpu-custom-plugin-monitor"
	op.EnableMetricsReporting = &defaultEnableMetricsReporting
	op.DefaultConditions = make([]Condition, 0)
	op.Rules = make([]*CustomRule, 0)
	return op
}

func getNPDCondition(afid AFID) Condition {
	return Condition{
		Type:       sanitizeNodeCondition(afid.ErrorType),
		Reason:     "AMDGPUIsUp",
		Message:    "AMDGPU is up",
		Transition: time.Time{},
	}
}

func getNPDCustomRule(id string, afid AFID) *CustomRule {
	return &CustomRule{
		Type:      Perm,
		Condition: sanitizeNodeCondition(afid.ErrorType),
		Reason:    "AMDGPUUnhealthy",
		Path:      "/var/lib/amd-metrics-exporter/amdgpuhealth",
		Args: []string{
			"query",
			"inband-ras-errors",
			fmt.Sprintf("-a=%s", id),
			"-s=CPER_SEVERITY_FATAL",
			// Decrement threshold by 1 because amdgpuhealth expects an exclusive upper bound
			fmt.Sprintf("-t=%d", afid.Threshold-1),
		},
		TimeoutString: &defaultGlobalTimeoutString,
	}
}

func writeToFile(filename string, data []byte) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", filename, err)
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("error writing to %s: %w", filename, err)
	}
	return nil
}

func generateNPDConfig(npdCustomPluginConfig *CustomPluginConfig) error {
	jsonBytes, err := json.MarshalIndent(npdCustomPluginConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal npd config: %w", err)
	}
	return writeToFile("npd-config.json", jsonBytes)
}

func generateConfigMap(conditionWorkflowMappings []ConditionWorkflowMapping, revision string) error {
	yamlbytes, err := yaml.Marshal(conditionWorkflowMappings)
	if err != nil {
		return fmt.Errorf("unable to marshal workflow mappings: %w", err)
	}
	defaultCfgMap := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "$CM_NAME$",
			Namespace: "$CM_NAMESPACE$",
		},
		Data: map[string]string{
			"Version":  revision,
			"workflow": string(yamlbytes),
		},
	}
	cfgMapBytes, err := k8syaml.Marshal(defaultCfgMap)
	if err != nil {
		return fmt.Errorf("unable to marshal configmap: %w", err)
	}
	return writeToFile("configmap.yaml", cfgMapBytes)
}

func main() {
	var (
		sagJsonPath  = flag.String("sag-json-path", "", "path to the SAG json that needs to be parsed")
		genConfigmap = flag.Bool("gen-configmap", false, "generate configmap for GPU-Operator")
		genNpdConfig = flag.Bool("gen-npd-config", false, "generate NPD custom plugin monitor config")
	)
	flag.Parse()

	afidField, err := parseAfidJSON(*sagJsonPath)
	if err != nil {
		fmt.Printf("unable to parse sag json. %v\n", err)
		return
	}
	conditionWorkflowMappings := make([]ConditionWorkflowMapping, 0)
	afidSaMap := AfidSaMap{
		Version: "1.0.0",
		Mapping: make(map[string]string),
	}
	npdCustomPluginConfig := getBaseNPDConfig()
	for id, afid := range afidField.Afid {
		i, err := strconv.Atoi(id)
		if err == nil && i > 10000 {
			continue
		}
		errSev := strings.TrimSpace(afid.ErrorSeverity)
		if errSev != "Fatal" && errSev != "Critical" {
			continue
		}
		afidSaMap.Mapping[id] = strconv.Itoa(afid.ServiceActionNum)
		cwmapping := ConditionWorkflowMapping{
			Afid:                     id,
			NodeCondition:            sanitizeNodeCondition(afid.ErrorType),
			WorkflowTemplate:         "default-template",
			ValidationTests:          afidField.getValidationTestsProfile(id),
			PhysicalActionNeeded:     "true",
			NotifyRemediationMessage: afidField.getNotifyRemediationMessage(id, ""),
			NotifyTestFailureMessage: afidField.getNotifyTestFailureMessage(id),
			RecoveryPolicy: RecoveryPolicyConfig{
				MaxAllowedRunsPerWindow: 3,
				WindowSize:              "15m",
			},
			Threshold: afid.Threshold,
		}
		if cwmapping.NotifyRemediationMessage == "" || cwmapping.NotifyRemediationMessage == "Rerun the known failing workload." {
			cwmapping.PhysicalActionNeeded = "false"
			cwmapping.NotifyRemediationMessage = ""
		}
		conditionWorkflowMappings = append(conditionWorkflowMappings, cwmapping)
		npdCustomPluginConfig.DefaultConditions = append(npdCustomPluginConfig.DefaultConditions, getNPDCondition(afid))
		npdCustomPluginConfig.Rules = append(npdCustomPluginConfig.Rules, getNPDCustomRule(id, afid))
	}
	if *genConfigmap {
		if err := generateConfigMap(conditionWorkflowMappings, afidField.Revision); err != nil {
			fmt.Printf("unable to generate configmap. Error: %v\n", err)
			return
		}
		fmt.Println("configmap.yaml is created")
	}
	if *genNpdConfig {
		if err := generateNPDConfig(npdCustomPluginConfig); err != nil {
			fmt.Printf("unable to generate npd config. Error: %v\n", err)
			return
		}
		fmt.Println("npd-config.json is created")
	}
}
