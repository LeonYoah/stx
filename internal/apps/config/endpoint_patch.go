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

package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

var (
	ErrHazelcastNetworkNotFound = errors.New("hazelcast.network not found / 未找到 hazelcast.network")
)

var hazelcastNetworkPath = []string{"hazelcast", "network"}

func HazelcastEndpointPort(configType ConfigType, deploymentMode string, ports []NodePortInfo) int {
	matches := func(node NodePortInfo, role string) bool {
		return node.Role == role || node.Role == "master/worker"
	}
	switch configType {
	case ConfigTypeHazelcastMaster:
		for _, node := range ports {
			if matches(node, "master") && node.HazelcastPort > 0 {
				return node.HazelcastPort
			}
		}
	case ConfigTypeHazelcastWorker:
		for _, node := range ports {
			if matches(node, "worker") {
				if node.WorkerPort > 0 {
					return node.WorkerPort
				}
				if node.HazelcastPort > 0 {
					return node.HazelcastPort
				}
			}
		}
	case ConfigTypeHazelcast:
		if strings.ToLower(strings.TrimSpace(deploymentMode)) != "hybrid" {
			return 0
		}
		for _, node := range ports {
			if node.HazelcastPort > 0 {
				return node.HazelcastPort
			}
		}
	}
	return 0
}

func applyHazelcastEndpoint(content string, configType ConfigType, deploymentMode string, hostIP string, ports []NodePortInfo) (string, error) {
	port := HazelcastEndpointPort(configType, deploymentMode, ports)
	if port <= 0 {
		return content, nil
	}
	publicAddress := ""
	if ip := strings.TrimSpace(hostIP); ip != "" {
		publicAddress = fmt.Sprintf("%s:%d", ip, port)
	}
	patched, _, err := PatchHazelcastEndpoint(content, publicAddress, port)
	return patched, err
}

func PatchHazelcastEndpoint(content string, publicAddress string, port int) (string, bool, error) {
	if port <= 0 && strings.TrimSpace(publicAddress) == "" {
		return content, false, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", false, fmt.Errorf("invalid hazelcast yaml: %w", err)
	}
	network, err := findYAMLMap(&doc, hazelcastNetworkPath)
	if err != nil {
		return "", false, err
	}
	changed := false
	if port > 0 {
		if setYAMLScalar(network, []string{"port", "port"}, fmt.Sprintf("%d", port), "!!int") {
			changed = true
		}
	}
	if address := strings.TrimSpace(publicAddress); address != "" {
		if setYAMLScalar(network, []string{"public-address"}, address, "!!str") {
			changed = true
		}
	}
	if !changed {
		return content, false, nil
	}
	out, err := encodeYAMLDocumentIndent(&doc, content)
	if err != nil {
		return "", false, err
	}
	if err := verifyEndpointPatch(content, out, publicAddress, port); err != nil {
		return "", false, err
	}
	return out, true, nil
}

func findYAMLMap(doc *yaml.Node, path []string) (*yaml.Node, error) {
	if doc == nil || len(doc.Content) == 0 {
		return nil, ErrHazelcastNetworkNotFound
	}
	current := doc.Content[0]
	for _, segment := range path {
		if current == nil || current.Kind != yaml.MappingNode {
			return nil, ErrHazelcastNetworkNotFound
		}
		var next *yaml.Node
		for i := 0; i+1 < len(current.Content); i += 2 {
			if current.Content[i].Value == segment {
				next = current.Content[i+1]
				break
			}
		}
		current = next
	}
	if current == nil || current.Kind != yaml.MappingNode {
		return nil, ErrHazelcastNetworkNotFound
	}
	return current, nil
}

func setYAMLScalar(mapping *yaml.Node, path []string, value string, tag string) bool {
	if len(path) == 0 || mapping == nil || mapping.Kind != yaml.MappingNode {
		return false
	}
	key := path[0]
	if len(path) == 1 {
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				target := mapping.Content[i+1]
				if target.Kind == yaml.ScalarNode && target.Value == value {
					return false
				}
				mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value, LineComment: target.LineComment}
				return true
			}
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
		return true
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return setYAMLScalar(mapping.Content[i+1], path[1:], value, tag)
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
	return setYAMLScalar(child, path[1:], value, tag)
}

func encodeYAMLDocumentIndent(doc *yaml.Node, original string) (string, error) {
	var out strings.Builder
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(detectIndent(original))
	if err := encoder.Encode(doc); err != nil {
		_ = encoder.Close()
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	return out.String(), nil
}

func verifyEndpointPatch(original, patched, publicAddress string, port int) error {
	var orig, next any
	if err := yaml.Unmarshal([]byte(original), &orig); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(patched), &next); err != nil {
		return fmt.Errorf("patched yaml no longer parses: %w", err)
	}
	if port > 0 {
		got := readDecodedPath(next, append(append([]string{}, hazelcastNetworkPath...), "port", "port"))
		if got != fmt.Sprintf("%d", port) && got != port && got != int64(port) && got != float64(port) {
			return fmt.Errorf("patched yaml port mismatch: got %v want %d", got, port)
		}
	}
	if address := strings.TrimSpace(publicAddress); address != "" {
		if got := readDecodedPath(next, append(append([]string{}, hazelcastNetworkPath...), "public-address")); got != address {
			return fmt.Errorf("patched yaml public-address mismatch: got %v want %s", got, address)
		}
	}
	deleteDecodedPath(orig, append(append([]string{}, hazelcastNetworkPath...), "port", "port"))
	deleteDecodedPath(next, append(append([]string{}, hazelcastNetworkPath...), "port", "port"))
	deleteDecodedPath(orig, append(append([]string{}, hazelcastNetworkPath...), "public-address"))
	deleteDecodedPath(next, append(append([]string{}, hazelcastNetworkPath...), "public-address"))
	if !reflect.DeepEqual(orig, next) {
		return fmt.Errorf("endpoint patch changed content outside network.port.port / public-address / 端点修补改动了 port 与 public-address 之外的内容")
	}
	return nil
}

func readDecodedPath(root any, path []string) any {
	current, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	for i, segment := range path {
		if i == len(path)-1 {
			return current[segment]
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

func deleteDecodedPath(root any, path []string) {
	current, ok := root.(map[string]any)
	if !ok {
		return
	}
	for i, segment := range path {
		if i == len(path)-1 {
			delete(current, segment)
			return
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
}
