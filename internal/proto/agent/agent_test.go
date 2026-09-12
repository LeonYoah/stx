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

// Package agent Protobuf 消息属性测试
package agent

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"google.golang.org/protobuf/proto"
)

// **Feature: stx-agent, Property 13: Protobuf Message Round-Trip**
// **Validates: Requirements 8.2, 8.3, 8.4, 8.5**
// 对于任何有效的 RegisterRequest、HeartbeatRequest、CommandRequest 或 CommandResponse 消息，
// 序列化为字节后再反序列化应产生等价的消息

// genSystemInfo 生成随机的 SystemInfo 消息
func genSystemInfo() gopter.Gen {
	return gopter.CombineGens(
		gen.Int32Range(1, 128),      // cpu_cores
		gen.Int64Range(1024, 1<<40), // total_memory (1KB to 1TB)
		gen.Int64Range(1024, 1<<50), // total_disk (1KB to 1PB)
		gen.AlphaString(),           // kernel_version
	).Map(func(vals []interface{}) *SystemInfo {
		return &SystemInfo{
			CpuCores:      vals[0].(int32),
			TotalMemory:   vals[1].(int64),
			TotalDisk:     vals[2].(int64),
			KernelVersion: vals[3].(string),
		}
	})
}

// genRegisterRequest 生成随机的 RegisterRequest 消息
func genRegisterRequest() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(), // agent_id
		gen.AlphaString(), // hostname
		gen.AlphaString(), // ip_address
		gen.OneConstOf("linux", "darwin", "windows"), // os_type
		gen.OneConstOf("amd64", "arm64"),             // arch
		gen.AlphaString(),                            // agent_version
		genSystemInfo(),                              // system_info
	).Map(func(vals []interface{}) *RegisterRequest {
		return &RegisterRequest{
			AgentId:      vals[0].(string),
			Hostname:     vals[1].(string),
			IpAddress:    vals[2].(string),
			OsType:       vals[3].(string),
			Arch:         vals[4].(string),
			AgentVersion: vals[5].(string),
			SystemInfo:   vals[6].(*SystemInfo),
		}
	})
}

// genResourceUsage 生成随机的 ResourceUsage 消息
func genResourceUsage() gopter.Gen {
	return gopter.CombineGens(
		gen.Float64Range(0, 100), // cpu_usage
		gen.Float64Range(0, 100), // memory_usage
		gen.Float64Range(0, 100), // disk_usage
		gen.Int64Range(0, 1<<40), // available_memory
		gen.Int64Range(0, 1<<50), // available_disk
	).Map(func(vals []interface{}) *ResourceUsage {
		return &ResourceUsage{
			CpuUsage:        vals[0].(float64),
			MemoryUsage:     vals[1].(float64),
			DiskUsage:       vals[2].(float64),
			AvailableMemory: vals[3].(int64),
			AvailableDisk:   vals[4].(int64),
		}
	})
}

// genProcessStatus 生成随机的 ProcessStatus 消息
func genProcessStatus() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),                               // name
		gen.Int32Range(1, 65535),                        // pid
		gen.OneConstOf("running", "stopped", "unknown"), // status
		gen.Int64Range(0, 86400*365),                    // uptime (up to 1 year in seconds)
		gen.Float64Range(0, 100),                        // cpu_usage
		gen.Int64Range(0, 1<<40),                        // memory_usage
	).Map(func(vals []interface{}) *ProcessStatus {
		return &ProcessStatus{
			Name:        vals[0].(string),
			Pid:         vals[1].(int32),
			Status:      vals[2].(string),
			Uptime:      vals[3].(int64),
			CpuUsage:    vals[4].(float64),
			MemoryUsage: vals[5].(int64),
		}
	})
}

// genHeartbeatRequest 生成随机的 HeartbeatRequest 消息
func genHeartbeatRequest() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),                   // agent_id
		gen.Int64Range(0, 1<<62),            // timestamp
		genResourceUsage(),                  // resource_usage
		gen.SliceOfN(3, genProcessStatus()), // processes (0-3 processes)
	).Map(func(vals []interface{}) *HeartbeatRequest {
		processes := vals[3].([]*ProcessStatus)
		return &HeartbeatRequest{
			AgentId:       vals[0].(string),
			Timestamp:     vals[1].(int64),
			ResourceUsage: vals[2].(*ResourceUsage),
			Processes:     processes,
		}
	})
}

// genCommandType 生成随机的 CommandType 枚举值
func genCommandType() gopter.Gen {
	return gen.OneConstOf(
		CommandType_COMMAND_TYPE_UNSPECIFIED,
		CommandType_PRECHECK,
		CommandType_INSTALL,
		CommandType_UNINSTALL,
		CommandType_UPGRADE,
		CommandType_START,
		CommandType_STOP,
		CommandType_RESTART,
		CommandType_STATUS,
		CommandType_COLLECT_LOGS,
		CommandType_JVM_DUMP,
		CommandType_THREAD_DUMP,
		CommandType_UPDATE_CONFIG,
		CommandType_ROLLBACK_CONFIG,
	)
}

// genStringMap 生成随机的 map[string]string
func genStringMap() gopter.Gen {
	return gen.MapOf(gen.AlphaString(), gen.AlphaString())
}

// genCommandRequest 生成随机的 CommandRequest 消息
func genCommandRequest() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),       // command_id
		genCommandType(),        // type
		genStringMap(),          // parameters
		gen.Int32Range(1, 3600), // timeout (1 second to 1 hour)
	).Map(func(vals []interface{}) *CommandRequest {
		return &CommandRequest{
			CommandId:  vals[0].(string),
			Type:       vals[1].(CommandType),
			Parameters: vals[2].(map[string]string),
			Timeout:    vals[3].(int32),
		}
	})
}

// genCommandStatus 生成随机的 CommandStatus 枚举值
func genCommandStatus() gopter.Gen {
	return gen.OneConstOf(
		CommandStatus_COMMAND_STATUS_UNSPECIFIED,
		CommandStatus_PENDING,
		CommandStatus_RUNNING,
		CommandStatus_SUCCESS,
		CommandStatus_FAILED,
		CommandStatus_CANCELLED,
	)
}

// genCommandResponse 生成随机的 CommandResponse 消息
func genCommandResponse() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString(),        // command_id
		genCommandStatus(),       // status
		gen.Int32Range(0, 100),   // progress
		gen.AlphaString(),        // output
		gen.AlphaString(),        // error
		gen.Int64Range(0, 1<<62), // timestamp
	).Map(func(vals []interface{}) *CommandResponse {
		return &CommandResponse{
			CommandId: vals[0].(string),
			Status:    vals[1].(CommandStatus),
			Progress:  vals[2].(int32),
			Output:    vals[3].(string),
			Error:     vals[4].(string),
			Timestamp: vals[5].(int64),
		}
	})
}

// TestProperty_RegisterRequestRoundTrip 测试 RegisterRequest 消息的序列化/反序列化往返一致性
func TestProperty_RegisterRequestRoundTrip(t *testing.T) {
	// **Feature: stx-agent, Property 13: Protobuf Message Round-Trip**
	// **Validates: Requirements 8.2**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("RegisterRequest round-trip serialization preserves data", prop.ForAll(
		func(msg *RegisterRequest) bool {
			// 序列化
			data, err := proto.Marshal(msg)
			if err != nil {
				return false
			}

			// 反序列化
			decoded := &RegisterRequest{}
			err = proto.Unmarshal(data, decoded)
			if err != nil {
				return false
			}

			// 验证等价性
			return proto.Equal(msg, decoded)
		},
		genRegisterRequest(),
	))

	properties.TestingRun(t)
}

// TestProperty_HeartbeatRequestRoundTrip 测试 HeartbeatRequest 消息的序列化/反序列化往返一致性
func TestProperty_HeartbeatRequestRoundTrip(t *testing.T) {
	// **Feature: stx-agent, Property 13: Protobuf Message Round-Trip**
	// **Validates: Requirements 8.3**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("HeartbeatRequest round-trip serialization preserves data", prop.ForAll(
		func(msg *HeartbeatRequest) bool {
			// 序列化
			data, err := proto.Marshal(msg)
			if err != nil {
				return false
			}

			// 反序列化
			decoded := &HeartbeatRequest{}
			err = proto.Unmarshal(data, decoded)
			if err != nil {
				return false
			}

			// 验证等价性
			return proto.Equal(msg, decoded)
		},
		genHeartbeatRequest(),
	))

	properties.TestingRun(t)
}

// TestProperty_CommandRequestRoundTrip 测试 CommandRequest 消息的序列化/反序列化往返一致性
func TestProperty_CommandRequestRoundTrip(t *testing.T) {
	// **Feature: stx-agent, Property 13: Protobuf Message Round-Trip**
	// **Validates: Requirements 8.4**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("CommandRequest round-trip serialization preserves data", prop.ForAll(
		func(msg *CommandRequest) bool {
			// 序列化
			data, err := proto.Marshal(msg)
			if err != nil {
				return false
			}

			// 反序列化
			decoded := &CommandRequest{}
			err = proto.Unmarshal(data, decoded)
			if err != nil {
				return false
			}

			// 验证等价性
			return proto.Equal(msg, decoded)
		},
		genCommandRequest(),
	))

	properties.TestingRun(t)
}

// TestProperty_CommandResponseRoundTrip 测试 CommandResponse 消息的序列化/反序列化往返一致性
func TestProperty_CommandResponseRoundTrip(t *testing.T) {
	// **Feature: stx-agent, Property 13: Protobuf Message Round-Trip**
	// **Validates: Requirements 8.5**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("CommandResponse round-trip serialization preserves data", prop.ForAll(
		func(msg *CommandResponse) bool {
			// 序列化
			data, err := proto.Marshal(msg)
			if err != nil {
				return false
			}

			// 反序列化
			decoded := &CommandResponse{}
			err = proto.Unmarshal(data, decoded)
			if err != nil {
				return false
			}

			// 验证等价性
			return proto.Equal(msg, decoded)
		},
		genCommandResponse(),
	))

	properties.TestingRun(t)
}
