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

package installer

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newContainerImageTestService(t *testing.T) *Service {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&ContainerImage{}))
	service := NewService(t.TempDir(), nil)
	service.SetDB(database)
	return service
}

func TestContainerImageCRUD(t *testing.T) {
	service := newContainerImageTestService(t)
	ctx := context.Background()

	created, err := service.CreateContainerImage(ctx, &ContainerImageRequest{
		Name: "st", Image: "apache/seatunnel:2.3.13", SeatunnelVersion: "2.3.13", IsDefault: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "IfNotPresent", created.ImagePullPolicy)
	assert.True(t, created.IsDefault)

	// 第二个默认镜像会挤掉第一个 / second default displaces the first
	second, err := service.CreateContainerImage(ctx, &ContainerImageRequest{
		Name: "st2", Image: "apache/seatunnel:2.3.14", IsDefault: true,
	})
	require.NoError(t, err)
	list, err := service.ListContainerImages(ctx)
	require.NoError(t, err)
	defaults := 0
	for _, item := range list.Items {
		if item.IsDefault {
			defaults++
			assert.Equal(t, second.ID, item.ID)
		}
	}
	assert.Equal(t, 1, defaults)

	updated, err := service.UpdateContainerImage(ctx, created.ID, &ContainerImageRequest{
		Name: "st-renamed", Image: "apache/seatunnel:2.3.13", Description: "d",
	})
	require.NoError(t, err)
	assert.Equal(t, "st-renamed", updated.Name)

	require.NoError(t, service.DeleteContainerImage(ctx, created.ID))
	assert.Error(t, service.DeleteContainerImage(ctx, created.ID))
}

func TestValidateContainerImageRequest(t *testing.T) {
	assert.Error(t, validateContainerImageRequest(&ContainerImageRequest{Image: "x:1"}))
	assert.Error(t, validateContainerImageRequest(&ContainerImageRequest{Name: "x", Image: "no-tag-image"}))
	assert.Error(t, validateContainerImageRequest(&ContainerImageRequest{Name: "x", Image: "a:1", ImagePullPolicy: "Bogus"}))
	req := &ContainerImageRequest{Name: "x", Image: "a:1"}
	assert.NoError(t, validateContainerImageRequest(req))
	assert.Equal(t, "IfNotPresent", req.ImagePullPolicy)
}
