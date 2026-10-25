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
// Package clustermembers 计算 SeaTunnel(Hazelcast) 集群 member-list 地址，供控制面与 Agent 共用，避免两处规则漂移。
// Package clustermembers computes SeaTunnel (Hazelcast) member-list addresses shared by the
// Control Plane and the Agent so that both sides follow one rule.
package clustermembers

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// 部署模式与角色取值，与集群模型及 Agent 安装参数保持一致。
// Deployment modes and roles, aligned with the cluster model and the Agent install params.
const (
	ModeHybrid    = "hybrid"
	ModeSeparated = "separated"

	RoleMaster       = "master"
	RoleWorker       = "worker"
	RoleMasterWorker = "master/worker"
	RoleHybrid       = "hybrid"

	// DefaultMasterPort 是 master / 混合节点的默认 Hazelcast 端口。
	// DefaultMasterPort is the default Hazelcast port of master and hybrid nodes.
	DefaultMasterPort = 5801
	// DefaultWorkerPort 是分离模式 worker 节点的默认 Hazelcast 端口。
	// DefaultWorkerPort is the default Hazelcast port of worker nodes in separated mode.
	DefaultWorkerPort = 5802
)

// Member 是参与成员地址计算的一个节点。
// Member is one node that takes part in the member-list computation.
type Member struct {
	// HostID 用于区分“同一主机多个角色”与“不同主机重复 IP”，未知时为 0。
	// HostID tells "one host with several roles" apart from "different hosts sharing an IP"; 0 when unknown.
	HostID uint
	// HostName 仅用于错误项展示。
	// HostName is only used for issue reporting.
	HostName string
	// HostIP 是节点可路由地址（IP 或主机名）。
	// HostIP is the routable node address (IP or host name).
	HostIP string
	// Role 为 master / worker / master/worker（hybrid）。
	// Role is master / worker / master/worker (hybrid).
	Role string
	// Port 是该节点的 Hazelcast 端口；分离模式 worker 节点即 worker 端口。为 0 时按模式与角色取默认值。
	// Port is the node's Hazelcast port (the worker port for separated-mode workers); 0 falls back to the
	// default for the mode and role.
	Port int
}

// DefaultPort 返回指定模式与角色的默认 Hazelcast 端口：混合模式与 master 为 5801，分离模式 worker 为 5802。
// DefaultPort returns the default Hazelcast port for a mode and role: 5801 for hybrid and masters,
// 5802 for separated-mode workers.
func DefaultPort(mode, role string) int {
	if normalizeMode(mode) == ModeSeparated && normalizeRole(role) == RoleWorker {
		return DefaultWorkerPort
	}
	return DefaultMasterPort
}

// FormatAddress 把主机与端口拼成 member-list 地址；裸 IPv6 会加方括号，其余输入保持 host:port 原样拼接。
// FormatAddress joins host and port into a member-list address; a bare IPv6 address is bracketed and every
// other input is joined as host:port verbatim.
func FormatAddress(host string, port int) string {
	host = strings.TrimSpace(host)
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// FormatAddresses 按输入顺序把一组主机格式化为同一端口的地址，不去重、不排序（Agent 安装器使用，保持旧行为）。
// FormatAddresses formats hosts with one port in input order, without dedup or sorting (used by the
// Agent installer to keep its historical output).
func FormatAddresses(hosts []string, port int) []string {
	result := make([]string, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, FormatAddress(host, port))
	}
	return result
}

// InstallerMemberList 按 Agent 安装器的历史规则生成 member-list：混合模式所有 master 地址使用 masterPort；
// 分离模式先 master（masterPort）后 worker（workerPort）。端口为 0 时取默认值；保持输入顺序且不去重，
// 地址为空时返回空切片，由调用方决定回退值。
// InstallerMemberList builds the member-list with the Agent installer's historical rule: in hybrid mode
// every master address uses masterPort; in separated mode masters (masterPort) come before workers
// (workerPort). Zero ports fall back to defaults; input order is kept without dedup, and an empty slice
// is returned when no address is given so the caller decides the fallback.
func InstallerMemberList(mode string, masterHosts, workerHosts []string, masterPort, workerPort int) []string {
	if masterPort == 0 {
		masterPort = DefaultMasterPort
	}
	if normalizeMode(mode) != ModeSeparated {
		return FormatAddresses(masterHosts, masterPort)
	}
	if workerPort == 0 {
		workerPort = DefaultWorkerPort
	}
	result := FormatAddresses(masterHosts, masterPort)
	return append(result, FormatAddresses(workerHosts, workerPort)...)
}

// Compute 计算集群完整 member-list：nodes 加上 add，再去掉 remove（按地址匹配），结果去重并稳定排序。
// 混合模式与分离模式都按每个节点自身端口生成 <ip>:<port>；分离模式 worker 节点的端口即 worker 端口。
// Compute returns the complete cluster member-list: nodes plus add, minus remove (matched by address),
// deduplicated and stably sorted. Both modes render <ip>:<port> from each node's own port; for
// separated-mode workers that port is the worker port.
func Compute(mode string, nodes, add, remove []Member) []string {
	removed := make(map[string]struct{}, len(remove))
	for _, member := range remove {
		if address, ok := memberAddress(mode, member); ok {
			removed[address] = struct{}{}
		}
	}

	seen := make(map[string]struct{}, len(nodes)+len(add))
	result := make([]string, 0, len(nodes)+len(add))
	for _, group := range [][]Member{nodes, add} {
		for _, member := range group {
			address, ok := memberAddress(mode, member)
			if !ok {
				continue
			}
			if _, skip := removed[address]; skip {
				continue
			}
			if _, dup := seen[address]; dup {
				continue
			}
			seen[address] = struct{}{}
			result = append(result, address)
		}
	}
	SortAddresses(result)
	return result
}

// SortAddresses 原地稳定排序地址：可解析的 IP 按数值排序并排在主机名之前，同主机按端口升序。
// SortAddresses sorts addresses in place: parseable IPs sort numerically before host names, and the same
// host sorts by ascending port.
func SortAddresses(addresses []string) {
	sort.SliceStable(addresses, func(i, j int) bool {
		return lessAddress(addresses[i], addresses[j])
	})
}

// IsLoopback 判断地址（可带端口）是否为回环：localhost、127.0.0.0/8、::1。
// IsLoopback reports whether an address (optionally with port) is loopback: localhost, 127.0.0.0/8, ::1.
func IsLoopback(address string) bool {
	host := strings.ToLower(hostOf(address))
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IssueCode 是成员地址校验问题的稳定编码，前端据此做国际化展示。
// IssueCode is a stable code of a member address issue; the frontend localizes it.
type IssueCode string

const (
	// IssueEmptyIP 节点没有地址。
	// IssueEmptyIP means a node has no address.
	IssueEmptyIP IssueCode = "empty_ip"
	// IssueLoopbackIP 节点地址是回环地址，其他主机无法连接。
	// IssueLoopbackIP means the node address is loopback and unreachable from other hosts.
	IssueLoopbackIP IssueCode = "loopback_ip"
	// IssueUnspecifiedIP 节点地址是 0.0.0.0 / ::，不可路由。
	// IssueUnspecifiedIP means the node address is 0.0.0.0 / :: and not routable.
	IssueUnspecifiedIP IssueCode = "unspecified_ip"
	// IssueDuplicateIP 多台不同主机使用同一地址（如多网卡或多机同 IP）。
	// IssueDuplicateIP means several different hosts share one address (multi-NIC or same IP on several hosts).
	IssueDuplicateIP IssueCode = "duplicate_ip"
)

// Issue 是一条成员地址校验问题。
// Issue is one member address validation problem.
type Issue struct {
	Code     IssueCode `json:"code"`
	HostIP   string    `json:"host_ip"`
	HostIDs  []uint    `json:"host_ids"`
	HostName string    `json:"host_name,omitempty"`
	Message  string    `json:"message"`
}

// Validate 检查成员地址是否可用于组建集群：空地址、回环、未指定地址，以及不同主机使用相同 IP。
// 同一主机（HostID 相同）的多个角色共用 IP 属于正常情况。返回值永不为 nil。
// Validate checks whether member addresses can form a cluster: empty, loopback and unspecified
// addresses, plus different hosts sharing one IP. Several roles on the same host (same HostID) sharing an
// IP are fine. The result is never nil.
func Validate(members []Member) []Issue {
	issues := make([]Issue, 0)
	reported := make(map[string]struct{})
	hostsByIP := make(map[string][]uint)
	namesByIP := make(map[string][]string)
	ipOrder := make([]string, 0)

	for _, member := range members {
		ip := strings.TrimSpace(member.HostIP)
		label := memberLabel(member)
		switch {
		case ip == "":
			key := fmt.Sprintf("empty:%d:%s", member.HostID, label)
			if _, ok := reported[key]; !ok {
				reported[key] = struct{}{}
				issues = append(issues, Issue{
					Code: IssueEmptyIP, HostIDs: []uint{member.HostID}, HostName: member.HostName,
					Message: fmt.Sprintf("node %s has no address / 节点 %s 没有地址", label, label),
				})
			}
			continue
		case IsLoopback(ip):
			key := fmt.Sprintf("loopback:%d:%s", member.HostID, ip)
			if _, ok := reported[key]; !ok {
				reported[key] = struct{}{}
				issues = append(issues, Issue{
					Code: IssueLoopbackIP, HostIP: ip, HostIDs: []uint{member.HostID}, HostName: member.HostName,
					Message: fmt.Sprintf("node %s uses loopback address %s, which other hosts cannot reach / 节点 %s 使用回环地址 %s，其他主机无法连接", label, ip, label, ip),
				})
			}
			continue
		case isUnspecified(ip):
			key := fmt.Sprintf("unspecified:%d:%s", member.HostID, ip)
			if _, ok := reported[key]; !ok {
				reported[key] = struct{}{}
				issues = append(issues, Issue{
					Code: IssueUnspecifiedIP, HostIP: ip, HostIDs: []uint{member.HostID}, HostName: member.HostName,
					Message: fmt.Sprintf("node %s uses unroutable address %s / 节点 %s 使用不可路由地址 %s", label, ip, label, ip),
				})
			}
			continue
		}

		normalized := strings.ToLower(ip)
		if _, ok := hostsByIP[normalized]; !ok {
			ipOrder = append(ipOrder, normalized)
		}
		if !containsUint(hostsByIP[normalized], member.HostID) {
			hostsByIP[normalized] = append(hostsByIP[normalized], member.HostID)
			namesByIP[normalized] = append(namesByIP[normalized], label)
		}
	}

	for _, ip := range ipOrder {
		hostIDs := hostsByIP[ip]
		if len(hostIDs) < 2 {
			continue
		}
		names := strings.Join(namesByIP[ip], ", ")
		issues = append(issues, Issue{
			Code: IssueDuplicateIP, HostIP: ip, HostIDs: append([]uint(nil), hostIDs...),
			Message: fmt.Sprintf("address %s is shared by different hosts (%s); fix the host IP or pin agent.ip / 地址 %s 被多台主机共用（%s），请修正主机 IP 或在 Agent 配置 agent.ip", ip, names, ip, names),
		})
	}
	return issues
}

// memberAddress 计算单个节点的地址；无地址时返回 false。
// memberAddress renders one node's address; returns false when the node has no address.
func memberAddress(mode string, member Member) (string, bool) {
	host := strings.TrimSpace(member.HostIP)
	if host == "" {
		return "", false
	}
	port := member.Port
	if port <= 0 {
		port = DefaultPort(mode, member.Role)
	}
	return FormatAddress(host, port), true
}

func normalizeMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func normalizeRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	if normalized == RoleHybrid {
		return RoleMasterWorker
	}
	return normalized
}

// hostOf 去掉地址中的端口与方括号，无法拆分时把整体视为主机。
// hostOf strips the port and brackets from an address; when it cannot be split the whole value is the host.
func hostOf(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(address); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(address, "[]")
}

// splitAddress 拆出主机与端口；无端口时端口为 0。
// splitAddress splits host and port; the port is 0 when absent.
func splitAddress(address string) (string, int) {
	address = strings.TrimSpace(address)
	if host, portText, err := net.SplitHostPort(address); err == nil {
		port, _ := strconv.Atoi(portText)
		return strings.Trim(host, "[]"), port
	}
	return strings.Trim(address, "[]"), 0
}

func lessAddress(left, right string) bool {
	leftHost, leftPort := splitAddress(left)
	rightHost, rightPort := splitAddress(right)
	leftIP := normalizedIP(leftHost)
	rightIP := normalizedIP(rightHost)
	switch {
	case leftIP != nil && rightIP != nil:
		if cmp := bytes.Compare(leftIP, rightIP); cmp != 0 {
			return cmp < 0
		}
	case leftIP != nil:
		return true
	case rightIP != nil:
		return false
	default:
		if leftHost != rightHost {
			return leftHost < rightHost
		}
	}
	if leftPort != rightPort {
		return leftPort < rightPort
	}
	return left < right
}

// normalizedIP 返回 16 字节形式的 IP，便于 IPv4/IPv6 统一比较。
// normalizedIP returns the 16-byte form so IPv4 and IPv6 compare uniformly.
func normalizedIP(host string) net.IP {
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return ip.To16()
}

func isUnspecified(address string) bool {
	ip := net.ParseIP(hostOf(address))
	return ip != nil && ip.IsUnspecified()
}

func memberLabel(member Member) string {
	if name := strings.TrimSpace(member.HostName); name != "" {
		return name
	}
	if member.HostID > 0 {
		return fmt.Sprintf("host#%d", member.HostID)
	}
	if ip := strings.TrimSpace(member.HostIP); ip != "" {
		return ip
	}
	return "unknown"
}

func containsUint(values []uint, target uint) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
