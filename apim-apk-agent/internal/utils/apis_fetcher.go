/*
 *  Copyright (c) 2024, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

/*
 * Package "synchronizer" contains artifacts relate to fetching APIs and
 * API related updates from the control plane event-hub.
 * This file contains functions to retrieve APIs and API updates.
 */

package synchronizer

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"archive/zip"
	"bytes"
	"strings"

	"github.com/wso2/product-apim-tooling/apim-apk-agent/config"
	logger "github.com/wso2/product-apim-tooling/apim-apk-agent/internal/loggers"
	"github.com/wso2/product-apim-tooling/apim-apk-agent/pkg/logging"
	sync "github.com/wso2/product-apim-tooling/apim-apk-agent/pkg/synchronizer"
	transformer "github.com/wso2/product-apim-tooling/apim-apk-agent/pkg/transformer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sclientUtil "github.com/wso2/product-apim-tooling/apim-apk-agent/internal/k8sClient"
	mapperUtil "github.com/wso2/product-apim-tooling/apim-apk-agent/internal/mapper"
)

func init() {
	conf, _ := config.ReadConfigs()
	sync.InitializeWorkerPool(conf.ControlPlane.RequestWorkerPool.PoolSize, conf.ControlPlane.RequestWorkerPool.QueueSizePerPool,
		conf.ControlPlane.RequestWorkerPool.PauseTimeAfterFailure, conf.Agent.TrustStore.Location,
		conf.ControlPlane.SkipSSLVerification, conf.ControlPlane.HTTPClient.RequestTimeOut, conf.ControlPlane.RetryInterval,
		conf.ControlPlane.ServiceURL, conf.ControlPlane.Username, conf.ControlPlane.Password)
}

// FetchAPIsOnEvent  will fetch API from control plane during the API Notification Event
func FetchAPIsOnEvent(conf *config.Config, apiUUID *string, k8sClient client.Client) (*[]string, error) {
	// Populate data from config.
	apis := make([]string, 0)
	envs := conf.ControlPlane.EnvironmentLabels

	// Create a channel for the byte slice (response from the APIs from control plane)
	c := make(chan sync.SyncAPIResponse)

	//Get API details.
	if apiUUID != nil {
		GetAPI(c, apiUUID, envs, sync.RuntimeArtifactEndpoint, true)
	} else {
		GetAPI(c, nil, envs, sync.RuntimeArtifactEndpoint, true)
	}
	data := <-c
	logger.LoggerUtils.Debugf("Receiving data for an API: %v", apiUUID)
	if data.Resp != nil {
		if data.Found {
			// Reading the root zip
			zipReader, err := zip.NewReader(bytes.NewReader(data.Resp), int64(len(data.Resp)))

			// apiFiles represents zipped API files fetched from API Manager
			apiFiles := make(map[string]*zip.File)
			// Read the .zip files within the root apis.zip and add apis to apiFiles array.
			for _, file := range zipReader.File {
				apiFiles[file.Name] = file
				logger.LoggerUtils.Debugf("API file found: " + file.Name)
				// Todo: Read the apis.zip and extract the api.zip,deployments.json
			}
			if err != nil {
				logger.LoggerUtils.Errorf("Error while reading zip: %v", err)
				return nil, err
			}
			deploymentJSON, exists := apiFiles["deployments.json"]
			if !exists {
				logger.LoggerUtils.Errorf("deployments.json not found")
				return nil, err
			}
			deploymentJSONBytes, err := transformer.ReadContent(deploymentJSON)
			if err != nil {
				logger.LoggerUtils.Errorf("Error while decoding the API Project Artifact: %v", err)
				return nil, err
			}
			deploymentDescriptor, err := transformer.ProcessDeploymentDescriptor(deploymentJSONBytes)
			if err != nil {
				logger.LoggerUtils.Errorf("Error while decoding the API Project Artifact: %v", err)
				return nil, err
			}
			apiDeployments := deploymentDescriptor.Data.Deployments
			if apiDeployments != nil {
				for _, apiDeployment := range *apiDeployments {
					apiZip, exists := apiFiles[apiDeployment.APIFile]
					if exists {
						artifact, decodingError := transformer.DecodeAPIArtifact(apiZip)
						if decodingError != nil {
							logger.LoggerUtils.Errorf("Error while decoding the API Project Artifact: %v", decodingError)
							return nil, err
						}

						apkConf, apiUUID, revisionID, configuredRateLimitPoliciesMap, endpointSecurityData, api, prodAIRL, sandAIRL, apkErr := transformer.GenerateAPKConf(artifact.APIJson, artifact.CertArtifact, artifact.Endpoints, apiDeployment.OrganizationID)
						if prodAIRL == nil {
							// Try to delete production AI ratelimit for this api
							k8sclientUtil.DeleteAIRatelimitPolicy(generateSHA1HexHash(api.Name, api.Version, "production"), k8sClient)
						}
						if sandAIRL == nil {
							// Try to delete production AI ratelimit for this api
							k8sclientUtil.DeleteAIRatelimitPolicy(generateSHA1HexHash(api.Name, api.Version, "sandbox"), k8sClient)
						}
						if apkErr != nil {
							logger.LoggerUtils.Errorf("Error while generating APK-Conf: %v", apkErr)
							return nil, err
						}
						logger.LoggerUtils.Debugf("APK Conf: %v", apkConf)
						certContainer := transformer.CertContainer{
							ClientCertObj:   artifact.CertMeta,
							EndpointCertObj: artifact.EndpointCertMeta,
							SecretData:      endpointSecurityData,
						}
						k8ResourceEndpoint := conf.DataPlane.K8ResourceEndpoint
						crResponse, err := transformer.GenerateCRs(apkConf, artifact.Schema, certContainer, k8ResourceEndpoint, apiDeployment.OrganizationID)
						if err != nil {
							logger.LoggerUtils.Errorf("Error occured in receiving the updated CRDs: %v", err)
							return nil, err
						}
						transformer.UpdateCRS(crResponse, apiDeployment.Environments, apiDeployment.OrganizationID, apiUUID, fmt.Sprint(revisionID), "namespace", configuredRateLimitPoliciesMap)
						mapperUtil.MapAndCreateCR(*crResponse, k8sClient)
						apis = append(apis, apiUUID)
						logger.LoggerUtils.Info("API applied successfully.\n")
					}
				}
				return &apis, nil
			}
		} else {
			logger.LoggerUtils.Info("API not found.")
			return &apis, nil
		}
	} else if data.ErrorCode == 204 {
		logger.LoggerUtils.Infof("No API Artifacts are available in the control plane for the envionments :%s",
			strings.Join(envs, ", "))
		return &[]string{}, nil
	} else if data.ErrorCode >= 400 && data.ErrorCode < 500 {
		logger.LoggerUtils.ErrorC(logging.ErrorDetails{
			Message:   fmt.Sprintf("Error occurred when retrieving APIs from control plane(unrecoverable error): %v", data.Err.Error()),
			Severity:  logging.CRITICAL,
			ErrorCode: 1106,
		})
		return nil, data.Err
	} else {
		// Keep the iteration still until all the envrionment response properly.
		logger.LoggerUtils.ErrorC(logging.ErrorDetails{
			Message:   fmt.Sprintf("Error occurred while fetching data from control plane: %v ..retrying..", data.Err),
			Severity:  logging.MINOR,
			ErrorCode: 1107,
		})
		//health.SetControlPlaneRestAPIStatus(false)
		sync.RetryFetchingAPIs(c, data, sync.RuntimeArtifactEndpoint, true)
	}
	logger.LoggerUtils.Info("Fetching API for an event is completed...")
	return nil, nil
}

// generateSHA1HexHash hashes the concatenated strings and returns the SHA-1 hash in base16 (hex) encoding.
func generateSHA1HexHash(name, version, env string) string {
	data := name + version + env
	hasher := sha1.New()
	hasher.Write([]byte(data))
	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

// GetAPI function calls the FetchAPIs() with relevant environment labels defined in the config.
func GetAPI(c chan sync.SyncAPIResponse, id *string, envs []string, endpoint string, sendType bool) {
	if len(envs) > 0 {
		// If the envrionment labels are present, call the controle plane with labels.
		logger.LoggerUtils.Debugf("Environment labels present: %v", envs)
		go sync.FetchAPIs(id, envs, c, endpoint, sendType)
	}
}
