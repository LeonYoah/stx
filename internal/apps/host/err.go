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

package host

import "errors"

// Error definitions for host management operations.
// 主机管理操作的错误定义。
var (
	// ErrHostNotFound indicates the requested host does not exist.
	// ErrHostNotFound 表示请求的主机不存在。
	ErrHostNotFound = errors.New("host: host not found")
	// ErrHostNameDuplicate indicates a host with the same name already exists.
	// ErrHostNameDuplicate 表示同名主机已存在。
	ErrHostNameDuplicate = errors.New("host: host name already exists")
	// ErrHostIPInvalid indicates the IP address format is invalid.
	// ErrHostIPInvalid 表示 IP 地址格式无效。
	ErrHostIPInvalid = errors.New("host: invalid IP address format")
	// ErrHostIPDuplicate indicates a host with the same IP already exists.
	// ErrHostIPDuplicate 表示相同 IP 的主机已存在。
	ErrHostIPDuplicate = errors.New("host: host IP address already exists")
	// ErrHostHasCluster indicates the host is associated with one or more clusters.
	// ErrHostHasCluster 表示主机关联了一个或多个集群。
	ErrHostHasCluster = errors.New("host: host is associated with clusters and cannot be deleted")
	// ErrHostHasRunningNode 主机上仍有运行中的集群节点，删除前须先停止并移出集群，避免孤儿进程。
	// ErrHostHasRunningNode means the host still has a running cluster node; stop and remove it
	// from the cluster before deleting the host so orphan processes are not left behind.
	ErrHostHasRunningNode = errors.New("host: host has running cluster nodes; stop them before delete / 主机上仍有运行中的集群节点，请先停止后再删除")
	// ErrHostNameEmpty indicates the host name is empty.
	// ErrHostNameEmpty 表示主机名为空。
	ErrHostNameEmpty = errors.New("host: host name cannot be empty")
	// ErrHostTypeInvalid indicates the host type is invalid.
	// ErrHostTypeInvalid 表示主机类型无效。
	ErrHostTypeInvalid = errors.New("host: invalid host type, must be bare_metal, docker, or kubernetes")
	// ErrDockerAPIURLInvalid indicates the Docker API URL format is invalid.
	// ErrDockerAPIURLInvalid 表示 Docker API URL 格式无效。
	ErrDockerAPIURLInvalid = errors.New("host: invalid Docker API URL format, must be tcp://host:port or unix:///path")
	// ErrK8sAPIURLInvalid indicates the Kubernetes API URL format is invalid.
	// ErrK8sAPIURLInvalid 表示 Kubernetes API URL 格式无效。
	ErrK8sAPIURLInvalid = errors.New("host: invalid Kubernetes API URL format, must be https://host:port")
	// ErrK8sCredentialsRequired indicates K8s credentials are required.
	// ErrK8sCredentialsRequired 表示需要 K8s 凭证。
	ErrK8sCredentialsRequired = errors.New("host: kubernetes host requires kubeconfig or token")
)

// Error codes for host management operations.
// 主机管理操作的错误代码。
const (
	ErrCodeHostNotFound           = 2001
	ErrCodeHostNameDuplicate      = 2002
	ErrCodeHostIPInvalid          = 2003
	ErrCodeHostHasCluster         = 2004
	ErrCodeHostTypeInvalid        = 2005
	ErrCodeDockerAPIURLInvalid    = 2006
	ErrCodeK8sAPIURLInvalid       = 2007
	ErrCodeK8sCredentialsRequired = 2008
	ErrCodeHostIPDuplicate        = 2009
)
