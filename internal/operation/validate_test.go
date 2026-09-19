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

package operation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryIsValid(t *testing.T) {
	require.NoError(t, Validate(Registry(), RouteExceptions()))
}

func TestValidateRejectsDuplicateOperationID(t *testing.T) {
	specs := Registry()
	duplicate := specs[0]
	duplicate.CommandPath = []string{"health", "duplicate"}
	duplicate.Route = "/api/v1/health/duplicate"

	err := Validate(append(specs, duplicate), nil)
	require.ErrorContains(t, err, "duplicate operation_id")
}

func TestValidateRejectsCommandConflict(t *testing.T) {
	specs := Registry()
	conflict := specs[0]
	conflict.ID = "health.duplicate"
	conflict.Route = "/api/v1/health/duplicate"

	err := Validate(append(specs, conflict), nil)
	require.ErrorContains(t, err, "command path")
}

func TestValidateRejectsInvalidRevision(t *testing.T) {
	specs := Registry()
	specs[0].Revision = 0

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "invalid revision")
}

func TestValidateRejectsGeneratedCLIWithoutSummary(t *testing.T) {
	specs := Registry()
	for index := range specs {
		if specs[index].GeneratedCLI {
			specs[index].Summary = ""
			err := Validate(specs, RouteExceptions())
			require.ErrorContains(t, err, "generated CLI command has no summary")
			return
		}
	}
	t.Fatal("登记表缺少生成式 CLI 操作 / registry has no generated CLI operation")
}

func TestRegistryContainsFirstGeneratedCLIReadBatch(t *testing.T) {
	expected := map[string]struct{}{
		"host.list": {}, "host.get": {}, "cluster.list": {}, "cluster.get": {},
		"cluster.node.list": {}, "cluster.status.get": {}, "config.cluster.list": {},
		"config.get": {}, "config.version.list": {},
	}
	actual := make(map[string]struct{})
	for _, spec := range Registry() {
		if spec.GeneratedCLI {
			actual[spec.ID] = struct{}{}
		}
	}
	require.Equal(t, expected, actual)
}

func TestValidateRejectsInvalidHelpExample(t *testing.T) {
	specs := Registry()
	specs[0].Example = "stx cluster list"

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "example must start")
}

func TestValidateRejectsInvalidOutputExample(t *testing.T) {
	specs := Registry()
	specs[0].OutputExample = `{"data": {}}`

	err := Validate(specs, nil)
	require.ErrorContains(t, err, "missing the required result envelope")
}

func TestValidateRejectsNormalRouteException(t *testing.T) {
	exceptions := []RouteException{{
		Method: "GET",
		Route:  "/api/v1/example",
		Mode:   ModeNormal,
		Reason: "example",
	}}

	err := Validate(nil, exceptions)
	require.ErrorContains(t, err, "invalid exception mode")
}
