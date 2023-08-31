package params

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/wso2/product-apim-tooling/import-export-cli/utils"
	"gopkg.in/yaml.v2"
)

// Configuration represents endpoint config
type Configuration struct {
	// RetryTimeOut for endpoint
	RetryTimeOut *int `yaml:"retryTimeOut" json:"retryTimeOut,string"`
	// RetryDelay for endpoint
	RetryDelay *int `yaml:"retryDelay" json:"retryDelay,string"`
	// Factor used for config
	Factor *int `yaml:"factor" json:"factor,string"`
}

// Advance endpoint configurations
type AdvanceEndpointConfiguration struct {
	// Timeout in milliseconds for endpoint
	TimeOutInMillis *int `yaml:"timeoutInMillis" json:"timeoutInMillis"`
}

// Endpoint details
type Endpoint struct {
	// Url of the endpoint
	Url                   *string            `yaml:"url" json:"url"`
	// Advance endpoint config of the endpoint
	AdvanceEndpointConfig *AdvanceEndpointConfiguration `yaml:"advanceEndpointConfig,omitempty" json:"advanceEndpointConfig,omitempty"`
	// Config of endpoint
	Config *Configuration `yaml:"config" json:"config"`
}

// EndpointData contains details about endpoints
type EndpointData struct {
	// Production endpoint
	Production *Endpoint `yaml:"production" json:"production_endpoints,omitempty"`
	// Sandbox endpoint
	Sandbox *Endpoint `yaml:"sandbox" json:"sandbox_endpoints,omitempty"`
}

// Cert stores certificate details
type Cert struct {
	// Host of the certificate
	Host string `yaml:"host" json:"hostName"`
	// Alias for certificate
	Alias string `yaml:"alias" json:"alias"`
	// Path for certificate file
	Path string `yaml:"path" json:"-"`
	// Certificate is used for internal purposes, it contains secret in base64
	Certificate string `json:"certificate"`
}

// Environment represents an api environment
type Environment struct {
	// Name of the environment
	Name string `yaml:"name"`
	// Endpoints contain details about endpoints in a configuration
	Endpoints *EndpointData `yaml:"endpoints"`
	// GatewayEnvironments contains environments that used to deploy API
	GatewayEnvironments []string `yaml:"gatewayEnvironments"`
	// Certs for environment
	Certs []Cert `yaml:"certs"`
}

// ApiParams represents environments defined in configuration file
type ApiParams struct {
	// Environments contains all environments in a configuration
	Environments []Environment `yaml:"environments"`
}

// APIEndpointConfig contains details about endpoints in an API
type APIEndpointConfig struct {
	// EPConfig is representing endpoint configuration
	EPConfig string `json:"endpointConfig"`
}

// LoadApiParams loads an configuration from a reader. It returns an error or a valid ApiParams
func LoadApiParams(r io.Reader) (*ApiParams, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	str, err := utils.EnvSubstitute(string(data))
	if err != nil {
		return nil, err
	}

	apiParams := &ApiParams{}
	err = yaml.Unmarshal([]byte(str), &apiParams)
	if err != nil {
		return nil, err
	}

	return apiParams, nil
}

// LoadApiParamsFromFile loads a configuration YAML file located in path. It returns an error or a valid ApiParams
func LoadApiParamsFromFile(path string) (*ApiParams, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	apiConfig, err := LoadApiParams(r)
	_ = r.Close()

	return apiConfig, err
}

// ExtractAPIEndpointConfig extracts API endpoint information from a slice of byte b
func ExtractAPIEndpointConfig(b []byte) (string, error) {
	apiConfig := &APIEndpointConfig{}
	err := json.Unmarshal(b, &apiConfig)
	if err != nil {
		return "", err
	}

	return apiConfig.EPConfig, err
}

// GetEnv returns the EndpointData associated for key in the ApiParams, if not found returns nil
func (config ApiParams) GetEnv(key string) *Environment {
	for index, env := range config.Environments {
		if env.Name == key {
			return &config.Environments[index]
		}
	}
	return nil
}
