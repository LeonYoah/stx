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

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type RuntimeStorageListItem struct {
	Path       string `json:"path,omitempty"`
	Name       string `json:"name,omitempty"`
	Directory  bool   `json:"directory,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

func (item *RuntimeStorageListItem) UnmarshalJSON(data []byte) error {
	type alias RuntimeStorageListItem
	var payload struct {
		alias
		SizeBytesCamel  *int64 `json:"sizeBytes,omitempty"`
		ModifiedAtCamel string `json:"modifiedAt,omitempty"`
		SizeBytesSnake  *int64 `json:"size_bytes,omitempty"`
		ModifiedAtSnake string `json:"modified_at,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		// Fall back to permissive decoding so future fields do not break parsing.
		var fallback struct {
			alias
			SizeBytesCamel  *int64 `json:"sizeBytes,omitempty"`
			ModifiedAtCamel string `json:"modifiedAt,omitempty"`
			SizeBytesSnake  *int64 `json:"size_bytes,omitempty"`
			ModifiedAtSnake string `json:"modified_at,omitempty"`
		}
		if err := json.Unmarshal(data, &fallback); err != nil {
			return err
		}
		*item = RuntimeStorageListItem(fallback.alias)
		if fallback.SizeBytesSnake != nil {
			item.SizeBytes = *fallback.SizeBytesSnake
		} else if fallback.SizeBytesCamel != nil {
			item.SizeBytes = *fallback.SizeBytesCamel
		}
		if strings.TrimSpace(fallback.ModifiedAtSnake) != "" {
			item.ModifiedAt = fallback.ModifiedAtSnake
		} else if strings.TrimSpace(fallback.ModifiedAtCamel) != "" {
			item.ModifiedAt = fallback.ModifiedAtCamel
		}
		return nil
	}
	*item = RuntimeStorageListItem(payload.alias)
	if payload.SizeBytesSnake != nil {
		item.SizeBytes = *payload.SizeBytesSnake
	} else if payload.SizeBytesCamel != nil {
		item.SizeBytes = *payload.SizeBytesCamel
	}
	if strings.TrimSpace(payload.ModifiedAtSnake) != "" {
		item.ModifiedAt = payload.ModifiedAtSnake
	} else if strings.TrimSpace(payload.ModifiedAtCamel) != "" {
		item.ModifiedAt = payload.ModifiedAtCamel
	}
	return nil
}

type RuntimeStorageListResult struct {
	ClusterID uint                     `json:"cluster_id"`
	Kind      string                   `json:"kind"`
	Path      string                   `json:"path,omitempty"`
	Items     []RuntimeStorageListItem `json:"items"`
}

func (s *Service) ListRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	kind installerapp.RuntimeStorageValidationKind,
	path string,
	recursive bool,
	limit int,
) (*RuntimeStorageListResult, error) {
	if s.agentSender == nil || s.hostProvider == nil {
		return nil, fmt.Errorf("agent sender or host provider is not configured")
	}
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, kind)
	if err != nil {
		return nil, err
	}
	if runtimeStorageValidationDisabled(kind, cfg) {
		return &RuntimeStorageListResult{ClusterID: clusterID, Kind: string(kind), Path: strings.TrimSpace(path), Items: make([]RuntimeStorageListItem, 0)}, nil
	}
	params := runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, kind, cfg.Checkpoint, cfg.IMAP, clusterObj)
	if strings.TrimSpace(path) != "" {
		params["path"] = strings.TrimSpace(path)
	}
	params["recursive"] = strconv.FormatBool(recursive)
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}
	success, output, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_list", params)
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if err == nil && result.Success {
		listResult := &RuntimeStorageListResult{ClusterID: clusterID, Kind: string(kind), Path: strings.TrimSpace(path), Items: make([]RuntimeStorageListItem, 0)}
		if result.Details != nil {
			if resolvedPath := strings.TrimSpace(result.Details["path"]); resolvedPath != "" {
				listResult.Path = resolvedPath
			}
			if rawItems := strings.TrimSpace(result.Details["items_json"]); rawItems != "" {
				_ = json.Unmarshal([]byte(rawItems), &listResult.Items)
			}
		}
		if limit > 0 && len(listResult.Items) > limit {
			listResult.Items = listResult.Items[:limit]
		}
		return listResult, nil
	}

	resolved, resolveErr := s.loadRuntimeStorageResolvedConfigFromNode(ctx, node, string(kind))
	if resolveErr == nil && resolved != nil && strings.EqualFold(strings.TrimSpace(resolved.StorageType), "S3") {
		items, resolvedPath, listErr := listS3RuntimeStorage(ctx, resolved, path, recursive, limit)
		if listErr == nil {
			return &RuntimeStorageListResult{
				ClusterID: clusterID,
				Kind:      string(kind),
				Path:      resolvedPath,
				Items:     items,
			}, nil
		}
	}

	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "runtime storage list failed"))
}

func listS3RuntimeStorage(
	ctx context.Context,
	cfg *runtimeStorageResolvedConfig,
	path string,
	recursive bool,
	limit int,
) ([]RuntimeStorageListItem, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("storage config is nil")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, "", fmt.Errorf("s3 endpoint or bucket is empty")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, "", fmt.Errorf("s3 credentials are unavailable")
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil {
		return nil, "", err
	}
	host := parsed.Host
	if host == "" {
		host = strings.TrimSpace(cfg.Endpoint)
	}
	bucket := sanitizeObjectStoreBucket(cfg.Bucket)
	prefixSource := cfg.Namespace
	if strings.TrimSpace(path) != "" {
		prefixSource = path
	}
	prefix := sanitizeObjectStorePrefix(prefixSource)
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       strings.EqualFold(parsed.Scheme, "https"),
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, "", err
	}
	items := make([]RuntimeStorageListItem, 0)
	seen := make(map[string]struct{})
	directorySizes := make(map[string]int64)
	options := minio.ListObjectsOptions{Prefix: prefix, Recursive: recursive, UseV1: true}
	for object := range client.ListObjects(ctx, bucket, options) {
		if object.Err != nil {
			return nil, "", object.Err
		}
		key := object.Key
		if key == "" {
			continue
		}
		if !recursive && key == prefix {
			continue
		}
		isDirectory := strings.HasSuffix(key, "/")
		if !recursive {
			relative := strings.TrimPrefix(key, prefix)
			relative = strings.TrimPrefix(relative, "/")
			if relative == "" {
				continue
			}
			parts := strings.SplitN(relative, "/", 2)
			if len(parts) > 1 {
				dirKey := strings.TrimSuffix(prefix, "/")
				if dirKey != "" {
					dirKey += "/"
				}
				dirKey += parts[0] + "/"
				directorySizes[dirKey] += object.Size
				if _, ok := seen[dirKey]; ok {
					continue
				}
				seen[dirKey] = struct{}{}
				name := strings.TrimSpace(parts[0])
				if name == "" {
					continue
				}
				items = append(items, RuntimeStorageListItem{
					Path:      dirKey,
					Name:      name,
					Directory: true,
				})
				if limit > 0 && len(items) >= limit {
					break
				}
				continue
			}
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		nameSource := strings.TrimSuffix(key, "/")
		name := pathpkg.Base(nameSource)
		if name == "." || name == "/" {
			name = strings.TrimSuffix(key, "/")
		}
		modifiedAt := object.LastModified.Format(time.RFC3339)
		if object.LastModified.IsZero() {
			modifiedAt = ""
		}
		items = append(items, RuntimeStorageListItem{
			Path:       key,
			Name:       name,
			Directory:  isDirectory,
			SizeBytes:  object.Size,
			ModifiedAt: modifiedAt,
		})
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	for index := range items {
		if items[index].Directory {
			items[index].SizeBytes = directorySizes[items[index].Path]
		}
	}
	resolvedPath := cfg.Bucket
	if prefix != "" {
		resolvedPath = strings.TrimRight(cfg.Bucket, "/") + "/" + prefix
	}
	return items, resolvedPath, nil
}
