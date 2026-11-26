/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"net/http"

	"github.com/spf13/cobra"
)

// containerImageFlags 镜像写命令的共用输入字段。
// containerImageFlags carries the shared input fields of image write commands.
type containerImageFlags struct {
	name             string
	image            string
	seatunnelVersion string
	pullPolicy       string
	pullSecrets      string
	isDefault        bool
	description      string
}

func addContainerImageFlags(command *cobra.Command, fields *containerImageFlags) {
	command.Flags().StringVar(&fields.name, "name", "", "Image display name")
	command.Flags().StringVar(&fields.image, "image", "", "Full image reference with tag or digest")
	command.Flags().StringVar(&fields.seatunnelVersion, "seatunnel-version", "", "SeaTunnel version the image carries")
	command.Flags().StringVar(&fields.pullPolicy, "image-pull-policy", "", "Always, IfNotPresent or Never")
	command.Flags().StringVar(&fields.pullSecrets, "image-pull-secrets", "", "Comma separated pull secret names")
	command.Flags().BoolVar(&fields.isDefault, "default", false, "Mark as default image")
	command.Flags().StringVar(&fields.description, "description", "", "Description")
}

func containerImageBody(fields *containerImageFlags) map[string]any {
	return map[string]any{
		"name":               fields.name,
		"image":              fields.image,
		"seatunnel_version":  fields.seatunnelVersion,
		"image_pull_policy":  fields.pullPolicy,
		"image_pull_secrets": fields.pullSecrets,
		"is_default":         fields.isDefault,
		"description":        fields.description,
	}
}

// newPackageImageCreateCommand stx package image create。
// newPackageImageCreateCommand registers a container image.
func newPackageImageCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerImageFlags
	command := &cobra.Command{
		Use:     "create",
		Short:   "Register a container image for Kubernetes deploys",
		Example: "stx package image create --name st-2.3.13 --image apache/seatunnel:2.3.13 --seatunnel-version 2.3.13 --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.image.create", &options,
				"registering a container image affects later Kubernetes deploy and upgrade choices")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/packages/images", containerImageBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.image.create", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerImageFlags(command, &fields)
	return command
}

// newPackageImageUpdateCommand stx package image update。
// newPackageImageUpdateCommand updates a registered container image.
func newPackageImageUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerImageFlags
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a registered container image",
		Example: "stx package image update 1 --name st-2.3.13 --image apache/seatunnel:2.3.13 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.image.update", &options,
				"updating a container image affects later Kubernetes deploy and upgrade choices")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut,
				"/api/v1/packages/images/"+args[0], containerImageBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.image.update", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerImageFlags(command, &fields)
	return command
}

// newPackageImageDeleteCommand stx package image delete。
// newPackageImageDeleteCommand deletes a registered container image.
func newPackageImageDeleteCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	command := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a registered container image",
		Example: "stx package image delete 1 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.image.delete", &options,
				"deleting a container image removes it from later Kubernetes deploy choices")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodDelete,
				"/api/v1/packages/images/"+args[0], nil, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.image.delete", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	return command
}
