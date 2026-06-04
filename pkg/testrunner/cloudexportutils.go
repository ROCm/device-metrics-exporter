/*
*
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
*
*/

package testrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	trproto "github.com/ROCm/device-metrics-exporter/pkg/testrunner/gen/testrunner"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/s3blob"
)

const (
	azureAccountName      = "AZURE_STORAGE_ACCOUNT"
	azureStorageKey       = "AZURE_STORAGE_KEY"
	awsAccessKeyId        = "AWS_ACCESS_KEY_ID"
	awsSecretAccessKey    = "AWS_SECRET_ACCESS_KEY"
	awsRegion             = "AWS_REGION"
	awsEndpointUrl        = "AWS_ENDPOINT_URL"
	cloudSecretPrefixPath = "/etc/logs-export-secrets"
)

func loadSecretKeyAsEnvVariable(secretName, key string) error {
	dat, err := os.ReadFile(filepath.Join(cloudSecretPrefixPath, secretName, strings.ToLower(key)))
	if err != nil {
		logger.Log.Printf("Secret %s does not contain %s key", secretName, strings.ToLower(key))
		return fmt.Errorf("Secret %s does not contain %s key", secretName, strings.ToLower(key))
	}
	err = os.Setenv(key, string(dat))
	if err != nil {
		logger.Log.Printf("Unable to set env variable %s. Error:%v", key, err)
		return fmt.Errorf("Unable to set env variable %s. Error:%v", key, err)
	}
	return nil
}

func setEnvVariablesFromSecretVolumeMount(cloudProvider, secretName string) error {
	var envs []string
	switch cloudProvider {
	case strings.ToLower(trproto.TestLogsExportConfig_Azure.String()):
		envs = []string{azureAccountName, azureStorageKey}
	case strings.ToLower(trproto.TestLogsExportConfig_Aws.String()):
		envs = []string{awsAccessKeyId, awsSecretAccessKey, awsRegion}
	default:
		logger.Log.Printf("cloud provider %s is not supported", cloudProvider)
		return fmt.Errorf("cloud provider %s is not supported", cloudProvider)
	}
	for _, env := range envs {
		err := loadSecretKeyAsEnvVariable(secretName, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func unsetEnvVariables(cloudProvider string) error {
	var envs []string
	switch cloudProvider {
	case strings.ToLower(trproto.TestLogsExportConfig_Azure.String()):
		envs = []string{azureAccountName, azureStorageKey}
	case strings.ToLower(trproto.TestLogsExportConfig_Aws.String()):
		envs = []string{awsAccessKeyId, awsSecretAccessKey, awsRegion}
	default:
		return fmt.Errorf("cloud provider %s is not supported", cloudProvider)
	}
	for _, env := range envs {
		os.Unsetenv(env)
	}
	return nil
}

func getAWSEndpointURL(secretName string) string {
	dat, err := os.ReadFile(filepath.Join(cloudSecretPrefixPath, secretName, strings.ToLower(awsEndpointUrl)))
	if err == nil {
		return string(dat)
	}
	return ""
}

func UploadFileToCloudBucket(cloudProvider, cloudBucket, cloudFolder, cloudFileName, localFilePath, secretName string) error {
	var cloudPrefix, url string
	err := setEnvVariablesFromSecretVolumeMount(cloudProvider, secretName)
	if err != nil {
		return err
	}
	defer func() {
		if err := unsetEnvVariables(cloudProvider); err != nil {
			logger.Log.Printf("failed to unset env variables: %v", err)
		}
	}()

	switch cloudProvider {
	case strings.ToLower(trproto.TestLogsExportConfig_Azure.String()):
		cloudPrefix = "azblob"
		url = fmt.Sprintf("%s://%s?prefix=%s/", cloudPrefix, cloudBucket, cloudFolder)
	case strings.ToLower(trproto.TestLogsExportConfig_Aws.String()):
		cloudPrefix = "s3"
		// for s3 compatible storage servers like minio, endpoint url is different from aws and passed in secret.
		endpointURL := getAWSEndpointURL(secretName)
		if endpointURL == "" {
			url = fmt.Sprintf("%s://%s?awssdk=v2&prefix=%s/", cloudPrefix, cloudBucket, cloudFolder)
		} else {
			url = fmt.Sprintf("%s://%s?endpoint=%s&disableSSL=true&s3ForcePathStyle=true&awssdk=v1&prefix=%s/", cloudPrefix, cloudBucket, endpointURL, cloudFolder)
		}
	default:
		return fmt.Errorf("cloud provider %s is not supported", cloudProvider)
	}
	logger.Log.Printf("url: %v", url)
	upload := func() error {
		ctx := context.Background()
		var bucket *blob.Bucket

		bucket, err := blob.OpenBucket(ctx, url)
		if err != nil {
			logger.Log.Printf("Unable to open connection to cloud blob storage url %s. Error %v", url, err)
			return err
		}
		defer bucket.Close()
		w, err := bucket.NewWriter(ctx, cloudFileName, nil)
		if err != nil {
			logger.Log.Printf("Unable to open blob writer. Error: %v", err)
			return err
		}
		fbytes, err := os.ReadFile(localFilePath)
		if err != nil {
			logger.Log.Printf("Unable to read the local file. Error: %v", err)
			return err
		}
		_, err = w.Write(fbytes)
		if err != nil {
			logger.Log.Printf("Unable to upload file to cloud. Error: %v", err)
			return err
		}
		if err := w.Close(); err != nil {
			logger.Log.Printf("Unable to close the writer. Error: %v", err)
			return err
		}
		return nil
	}
	// retry upload 3 times before marking it as failed
	for i := 0; i < 3; i++ {
		err = upload()
		if err == nil {
			break
		}
	}
	return err
}
