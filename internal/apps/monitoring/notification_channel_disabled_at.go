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

package monitoring

import (
	"time"

	"gorm.io/gorm"
)

// BeforeSave 维护 DisabledAt：启用时清空；从启用变为停用时记下停用时间，已停用的渠道再次保存时保留原时间。
// BeforeSave maintains DisabledAt: cleared while enabled; set when the channel becomes disabled and kept on later saves
// of an already disabled channel.
func (c *NotificationChannel) BeforeSave(_ *gorm.DB) error {
	if c.Enabled {
		c.DisabledAt = nil
		return nil
	}
	if c.DisabledAt == nil {
		now := time.Now().UTC()
		c.DisabledAt = &now
	}
	return nil
}

// disabledSince 返回渠道停用的起始时间；老数据没有 DisabledAt 时退回 UpdatedAt。
// disabledSince returns when the channel was disabled; legacy rows without DisabledAt fall back to UpdatedAt.
func (c *NotificationChannel) disabledSince() time.Time {
	if c.DisabledAt != nil {
		return c.DisabledAt.UTC()
	}
	return c.UpdatedAt.UTC()
}
