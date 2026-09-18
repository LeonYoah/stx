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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultOutput  = "json"
	DefaultTimeout = 30 * time.Second
)

var (
	ErrNamespaceNameRequired = errors.New("namespace name is required")
	ErrNamespaceNotFound     = errors.New("namespace not found")
	ErrNoCurrentNamespace    = errors.New("current namespace is not configured")
)

// File 表示 STX CLI 的本地配置文件。
// File represents the local STX CLI configuration file.
type File struct {
	CurrentNamespace string               `yaml:"current_namespace,omitempty"`
	Namespaces       map[string]Namespace `yaml:"namespaces,omitempty"`
}

// Namespace 保存一个远端 STX 服务及其本地凭据。
// Namespace stores one remote STX server and its local credentials.
type Namespace struct {
	Server         string `yaml:"server,omitempty"`
	Token          string `yaml:"token,omitempty"`
	TokenExpiresAt string `yaml:"token_expires_at,omitempty"`
	Output         string `yaml:"output,omitempty"`
	Timeout        string `yaml:"timeout,omitempty"`
}

// NamedNamespace 是带名称和当前状态的命名空间视图。
// NamedNamespace is a namespace view with its name and current status.
type NamedNamespace struct {
	Name    string
	Current bool
	Server  string
}

// Overrides 表示高于配置文件优先级的命令参数。
// Overrides represents command arguments that take precedence over the config file.
type Overrides struct {
	Namespace string
	Server    string
	Token     string
	Output    string
	Timeout   string
}

// Resolved 表示应用命令参数、环境变量和默认值后的有效配置。
// Resolved represents effective configuration after command arguments, environment variables, and defaults are applied.
type Resolved struct {
	Name           string
	Server         string
	Token          string
	TokenExpiresAt string
	Output         string
	Timeout        time.Duration
}

// Store 管理一个 CLI 配置文件。
// Store manages one CLI configuration file.
type Store struct {
	path   string
	getenv func(string) string
}

// DefaultPath 返回默认 CLI 配置文件路径。
// DefaultPath returns the default CLI configuration file path.
func DefaultPath() (string, error) {
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "stx", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "stx", "config.yaml"), nil
}

// NewStore 创建指定路径的配置存储。
// NewStore creates a configuration store at the specified path.
func NewStore(path string) *Store {
	return &Store{path: path, getenv: os.Getenv}
}

// NewDefaultStore 创建默认路径的配置存储。
// NewDefaultStore creates a configuration store at the default path.
func NewDefaultStore() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewStore(path), nil
}

// Path 返回配置文件路径。
// Path returns the configuration file path.
func (s *Store) Path() string {
	return s.path
}

// Load 读取配置；文件不存在时返回空配置。
// Load reads configuration and returns an empty configuration when the file does not exist.
func (s *Store) Load() (*File, error) {
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return newFile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read CLI config %q: %w", s.path, err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return nil, fmt.Errorf("secure CLI config %q: %w", s.path, err)
	}

	result := newFile()
	if len(content) == 0 {
		return result, nil
	}
	if err := yaml.Unmarshal(content, result); err != nil {
		return nil, fmt.Errorf("parse CLI config %q: %w", s.path, err)
	}
	if result.Namespaces == nil {
		result.Namespaces = make(map[string]Namespace)
	}
	return result, nil
}

// Save 以受限权限原子保存配置。
// Save atomically persists configuration with restricted permissions.
func (s *Store) Save(file *File) error {
	if file == nil {
		file = newFile()
	}
	if file.Namespaces == nil {
		file.Namespaces = make(map[string]Namespace)
	}

	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create CLI config directory %q: %w", directory, err)
	}
	if filepath.Clean(directory) != "." {
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("secure CLI config directory %q: %w", directory, err)
		}
	}

	content, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode CLI config: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary CLI config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary CLI config: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary CLI config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary CLI config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary CLI config: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace CLI config %q: %w", s.path, err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("secure CLI config %q: %w", s.path, err)
	}
	return nil
}

// List 返回按名称排序且不包含令牌的命名空间元数据。
// List returns namespace metadata sorted by name without tokens.
func (s *Store) List() ([]NamedNamespace, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(file.Namespaces))
	for name := range file.Namespaces {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]NamedNamespace, 0, len(names))
	for _, name := range names {
		namespace := file.Namespaces[name]
		result = append(result, NamedNamespace{
			Name:    name,
			Current: name == file.CurrentNamespace,
			Server:  namespace.Server,
		})
	}
	return result, nil
}

// Get 返回指定命名空间；名称为空时使用当前命名空间。
// Get returns a named namespace, using the current namespace when name is empty.
func (s *Store) Get(name string) (string, Namespace, error) {
	file, err := s.Load()
	if err != nil {
		return "", Namespace{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(file.CurrentNamespace)
	}
	if name == "" {
		return "", Namespace{}, ErrNoCurrentNamespace
	}
	namespace, ok := file.Namespaces[name]
	if !ok {
		return "", Namespace{}, fmt.Errorf("%w: %s", ErrNamespaceNotFound, name)
	}
	return name, namespace, nil
}

// Upsert 新增或更新命名空间，并可将其设为当前命名空间。
// Upsert creates or updates a namespace and can make it current.
func (s *Store) Upsert(name string, namespace Namespace, makeCurrent bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNamespaceNameRequired
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	file.Namespaces[name] = namespace
	if makeCurrent || file.CurrentNamespace == "" {
		file.CurrentNamespace = name
	}
	return s.Save(file)
}

// Use 将已存在的命名空间设为当前命名空间。
// Use selects an existing namespace as current.
func (s *Store) Use(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNamespaceNameRequired
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	if _, ok := file.Namespaces[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNamespaceNotFound, name)
	}
	file.CurrentNamespace = name
	return s.Save(file)
}

// Delete 删除命名空间；删除当前项后选择名称排序后的第一项。
// Delete removes a namespace and selects the first sorted remaining name when deleting the current one.
func (s *Store) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNamespaceNameRequired
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	if _, ok := file.Namespaces[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNamespaceNotFound, name)
	}
	delete(file.Namespaces, name)
	if file.CurrentNamespace == name {
		file.CurrentNamespace = firstNamespaceName(file.Namespaces)
	}
	return s.Save(file)
}

// Resolve 按命令参数、环境变量、当前命名空间和默认值解析有效配置。
// Resolve applies command arguments, environment variables, current namespace values, and defaults in priority order.
func (s *Store) Resolve(overrides Overrides) (Resolved, error) {
	file, err := s.Load()
	if err != nil {
		return Resolved{}, err
	}

	name := firstNonEmpty(overrides.Namespace, s.getenv("STX_NAMESPACE"), file.CurrentNamespace)
	stored := Namespace{}
	if name != "" {
		var ok bool
		stored, ok = file.Namespaces[name]
		if !ok && overrides.Server == "" && s.getenv("STX_SERVER") == "" {
			return Resolved{}, fmt.Errorf("%w: %s", ErrNamespaceNotFound, name)
		}
	}

	timeoutText := firstNonEmpty(overrides.Timeout, s.getenv("STX_TIMEOUT"), stored.Timeout, DefaultTimeout.String())
	timeout, err := time.ParseDuration(timeoutText)
	if err != nil || timeout <= 0 {
		return Resolved{}, fmt.Errorf("invalid timeout %q", timeoutText)
	}
	return Resolved{
		Name:           name,
		Server:         firstNonEmpty(overrides.Server, s.getenv("STX_SERVER"), stored.Server),
		Token:          firstNonEmpty(overrides.Token, s.getenv("STX_TOKEN"), stored.Token),
		TokenExpiresAt: stored.TokenExpiresAt,
		Output:         firstNonEmpty(overrides.Output, s.getenv("STX_OUTPUT"), stored.Output, DefaultOutput),
		Timeout:        timeout,
	}, nil
}

func newFile() *File {
	return &File{Namespaces: make(map[string]Namespace)}
}

func firstNamespaceName(namespaces map[string]Namespace) string {
	names := make([]string, 0, len(namespaces))
	for name := range namespaces {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
