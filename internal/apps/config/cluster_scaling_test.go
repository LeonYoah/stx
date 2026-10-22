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
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/db"
)

// scalingAgentClient 模拟各主机磁盘上的配置文件；offline 主机的拉取与推送都会失败。
// scalingAgentClient simulates config files on each host's disk; pulls and pushes to offline hosts fail.
type scalingAgentClient struct {
	files   map[uint]map[ConfigType]string
	offline map[uint]bool
	pushes  []string
	pulls   int
}

func newScalingAgentClient() *scalingAgentClient {
	return &scalingAgentClient{files: map[uint]map[ConfigType]string{}, offline: map[uint]bool{}}
}

func (c *scalingAgentClient) setFile(hostID uint, configType ConfigType, content string) {
	if c.files[hostID] == nil {
		c.files[hostID] = map[ConfigType]string{}
	}
	c.files[hostID][configType] = content
}

func (c *scalingAgentClient) PullConfig(_ context.Context, hostID uint, _ string, configType ConfigType) (string, error) {
	c.pulls++
	if c.offline[hostID] {
		return "", errors.New("agent not connected")
	}
	content, ok := c.files[hostID][configType]
	if !ok {
		return "", fmt.Errorf("file %s not found", configType)
	}
	return content, nil
}

func (c *scalingAgentClient) PushConfig(_ context.Context, hostID uint, _ string, configType ConfigType, content string) error {
	if c.offline[hostID] {
		return errors.New("agent not connected")
	}
	c.setFile(hostID, configType, content)
	c.pushes = append(c.pushes, fmt.Sprintf("%d:%s", hostID, configType))
	return nil
}

func newScalingTestService(t *testing.T) (*Service, *gorm.DB, *scalingAgentClient) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:scaling_"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&Config{}, &ConfigVersion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	agent := newScalingAgentClient()
	service := NewService(NewRepository(database), stubHostProvider{}, &testNodeInfoProvider{installDir: "/opt/seatunnel"}, agent)
	return service, database, agent
}

func hazelcastWithMembers(members ...string) string {
	var b strings.Builder
	b.WriteString("hazelcast:\n  cluster-name: seatunnel\n  network:\n    join:\n      tcp-ip:\n        enabled: true\n        member-list:\n")
	for _, member := range members {
		b.WriteString("          - " + member + "\n")
	}
	b.WriteString("    port:\n      port: 5801\n")
	return b.String()
}

const (
	scalingLog4j2PerJob  = "rootLogger.appenderRef.file.ref = routingAppender\n"
	scalingLog4j2Default = "rootLogger.appenderRef.file.ref = fileAppender\n"
)

func mustTemplate(t *testing.T, database *gorm.DB, clusterID uint, configType ConfigType) *Config {
	t.Helper()
	var config Config
	if err := database.Where("cluster_id = ? AND host_id IS NULL AND config_type = ?", clusterID, configType).First(&config).Error; err != nil {
		t.Fatalf("load template %s: %v", configType, err)
	}
	return &config
}

func mustNodeConfig(t *testing.T, database *gorm.DB, clusterID, hostID uint, configType ConfigType) *Config {
	t.Helper()
	var config Config
	if err := database.Where("cluster_id = ? AND host_id = ? AND config_type = ?", clusterID, hostID, configType).First(&config).Error; err != nil {
		t.Fatalf("load node config %d/%s: %v", hostID, configType, err)
	}
	return &config
}

func countVersions(t *testing.T, database *gorm.DB, configID uint) int64 {
	t.Helper()
	var count int64
	if err := database.Model(&ConfigVersion{}).Where("config_id = ?", configID).Count(&count).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	return count
}

func TestEnsureClusterConfigsForHostDoesNotOverwriteExistingTemplate(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	agent.setFile(1, ConfigTypeHazelcast, hazelcastWithMembers("10.66.0.2:5801"))
	agent.setFile(1, ConfigTypeLog4j2, scalingLog4j2PerJob)
	agent.setFile(2, ConfigTypeHazelcast, hazelcastWithMembers("10.66.0.2:5801", "10.66.0.1:5801"))
	agent.setFile(2, ConfigTypeLog4j2, scalingLog4j2Default)

	// 首节点：模板不存在，由节点文件创建。
	// First node: templates are missing and get created from the node files.
	if err := service.EnsureClusterConfigsForHost(ctx, 1, 1, "/opt/seatunnel", 0); err != nil {
		t.Fatalf("ensure host 1: %v", err)
	}
	tpl := mustTemplate(t, database, 1, ConfigTypeLog4j2)
	if tpl.Version != 1 || tpl.Content.String() != scalingLog4j2PerJob {
		t.Fatalf("template after first node = v%d %q", tpl.Version, tpl.Content.String())
	}

	// 后续节点：模板已存在，只建该主机配置，模板版本与内容不变。
	// Later node: templates exist, only the host config is created; template version/content are unchanged.
	if err := service.EnsureClusterConfigsForHost(ctx, 1, 2, "/opt/seatunnel", 0); err != nil {
		t.Fatalf("ensure host 2: %v", err)
	}
	for _, configType := range []ConfigType{ConfigTypeLog4j2, ConfigTypeHazelcast} {
		tpl := mustTemplate(t, database, 1, configType)
		if tpl.Version != 1 {
			t.Fatalf("template %s version bumped to %d", configType, tpl.Version)
		}
		if countVersions(t, database, tpl.ID) != 1 {
			t.Fatalf("template %s gained versions", configType)
		}
	}
	if got := mustTemplate(t, database, 1, ConfigTypeLog4j2).Content.String(); got != scalingLog4j2PerJob {
		t.Fatalf("per_job log4j2 template was overwritten: %q", got)
	}
	node2 := mustNodeConfig(t, database, 1, 2, ConfigTypeLog4j2)
	if node2.Content.String() != scalingLog4j2Default || node2.Version != 1 {
		t.Fatalf("host 2 config = v%d %q", node2.Version, node2.Content.String())
	}
	if len(agent.pushes) != 0 {
		t.Fatalf("ensure must not push to nodes, got %v", agent.pushes)
	}
}

func TestInitClusterConfigsStillOverwritesTemplate(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	agent.setFile(1, ConfigTypeLog4j2, scalingLog4j2PerJob)
	agent.setFile(2, ConfigTypeLog4j2, scalingLog4j2Default)

	if err := service.InitClusterConfigs(ctx, 1, 1, "/opt/seatunnel", 0); err != nil {
		t.Fatalf("init host 1: %v", err)
	}
	if err := service.InitClusterConfigs(ctx, 1, 2, "/opt/seatunnel", 0); err != nil {
		t.Fatalf("init host 2: %v", err)
	}
	tpl := mustTemplate(t, database, 1, ConfigTypeLog4j2)
	if tpl.Version != 2 || tpl.Content.String() != scalingLog4j2Default {
		t.Fatalf("explicit init should overwrite the template, got v%d %q", tpl.Version, tpl.Content.String())
	}
	if countVersions(t, database, tpl.ID) != 2 {
		t.Fatalf("expected 2 template versions")
	}
}

func TestRegisterHostConfigsFromNodeNeverTouchesTemplatesOrDisk(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	tpl := &Config{ClusterID: 1, ConfigType: ConfigTypeLog4j2, FilePath: GetConfigFilePath(ConfigTypeLog4j2), Content: scalingLog4j2PerJob, Version: 3}
	if err := database.Create(tpl).Error; err != nil {
		t.Fatal(err)
	}
	agent.setFile(2, ConfigTypeLog4j2, scalingLog4j2Default)
	agent.setFile(2, ConfigTypeSeatunnel, syncTestOld)

	result, err := service.RegisterHostConfigsFromNode(ctx, 1, 2, "/opt/seatunnel", 7)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.CreatedCount != 2 || result.Partial() {
		t.Fatalf("unexpected register result: %+v", result)
	}
	for _, item := range result.Items {
		if item.IsTemplate {
			t.Fatalf("register must not report template changes: %+v", item)
		}
	}
	if got := mustTemplate(t, database, 1, ConfigTypeLog4j2); got.Version != 3 || got.Content.String() != scalingLog4j2PerJob {
		t.Fatalf("template changed: v%d %q", got.Version, got.Content.String())
	}
	var seatunnelTemplates int64
	database.Model(&Config{}).Where("cluster_id = ? AND host_id IS NULL AND config_type = ?", 1, ConfigTypeSeatunnel).Count(&seatunnelTemplates)
	if seatunnelTemplates != 0 {
		t.Fatal("register must not create missing templates")
	}
	if mustNodeConfig(t, database, 1, 2, ConfigTypeLog4j2).Content.String() != scalingLog4j2Default {
		t.Fatal("host config should keep the node content")
	}
	if len(agent.pushes) != 0 {
		t.Fatalf("register must not write to the node, got %v", agent.pushes)
	}

	// match_template 如实反映差异。
	// match_template reports the difference faithfully.
	infos, err := service.GetByCluster(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		if !info.IsTemplate && info.ConfigType == ConfigTypeLog4j2 && info.MatchTemplate {
			t.Fatal("expected match_template=false for the registered log4j2 config")
		}
	}
}

func seedHybridMemberConfigs(t *testing.T, database *gorm.DB, hosts ...uint) {
	t.Helper()
	original := hazelcastWithMembers("localhost")
	records := []*Config{
		{ClusterID: 1, ConfigType: ConfigTypeHazelcast, FilePath: GetConfigFilePath(ConfigTypeHazelcast), Content: db.ScriptText(original), Version: 1},
		{ClusterID: 1, ConfigType: ConfigTypeHazelcastMaster, FilePath: GetConfigFilePath(ConfigTypeHazelcastMaster), Content: db.ScriptText(original), Version: 1},
		{ClusterID: 1, ConfigType: ConfigTypeSeatunnel, FilePath: GetConfigFilePath(ConfigTypeSeatunnel), Content: syncTestOld, Version: 1},
	}
	for _, h := range hosts {
		hostID := h
		records = append(records, &Config{ClusterID: 1, HostID: &hostID, ConfigType: ConfigTypeHazelcast, FilePath: GetConfigFilePath(ConfigTypeHazelcast), Content: db.ScriptText(original), Version: 1})
	}
	for _, record := range records {
		if err := database.Create(record).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestPatchMemberListHybridPatchesTemplateAndHostsOnly(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	seedHybridMemberConfigs(t, database, 1, 2)
	members := []string{"10.66.0.1:5801", "10.66.0.2:5801"}

	result, err := service.PatchMemberList(ctx, 1, "hybrid", members, "scale:42", 9)
	if err != nil {
		t.Fatalf("PatchMemberList: %v", err)
	}
	if result.UpdatedCount != 3 || result.Partial() || len(result.Items) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !result.Items[0].IsTemplate {
		t.Fatalf("template must be processed first: %+v", result.Items[0])
	}
	if !reflect.DeepEqual(result.Members, members) {
		t.Fatalf("result members = %v", result.Members)
	}

	expected := hazelcastWithMembers(members...)
	tpl := mustTemplate(t, database, 1, ConfigTypeHazelcast)
	if tpl.Content.String() != expected || tpl.Version != 2 {
		t.Fatalf("template = v%d\n%s", tpl.Version, tpl.Content.String())
	}
	var latest ConfigVersion
	database.Where("config_id = ?", tpl.ID).Order("version DESC").First(&latest)
	if !strings.Contains(latest.Comment, "scale:42") {
		t.Fatalf("version comment should carry opRef, got %q", latest.Comment)
	}
	// 混合模式不修补未使用的 hazelcast-master.yaml，也不碰 seatunnel.yaml。
	// Hybrid mode leaves the unused hazelcast-master.yaml and seatunnel.yaml alone.
	if got := mustTemplate(t, database, 1, ConfigTypeHazelcastMaster); got.Version != 1 {
		t.Fatalf("hazelcast-master.yaml template should be untouched in hybrid mode, got v%d", got.Version)
	}
	if got := mustTemplate(t, database, 1, ConfigTypeSeatunnel); got.Version != 1 {
		t.Fatalf("seatunnel.yaml template should be untouched, got v%d", got.Version)
	}
	for _, hostID := range []uint{1, 2} {
		node := mustNodeConfig(t, database, 1, hostID, ConfigTypeHazelcast)
		if node.Content.String() != expected || node.Version != 2 {
			t.Fatalf("host %d config = v%d", hostID, node.Version)
		}
		if agent.files[hostID][ConfigTypeHazelcast] != expected {
			t.Fatalf("host %d disk was not written: %q", hostID, agent.files[hostID][ConfigTypeHazelcast])
		}
	}

	// 再次执行：内容一致，不产生新版本、不推送。
	// Running again: content matches, no new version and no push.
	pushesBefore := len(agent.pushes)
	again, err := service.PatchMemberList(ctx, 1, "hybrid", members, "scale:43", 9)
	if err != nil {
		t.Fatalf("second PatchMemberList: %v", err)
	}
	if again.UnchangedCount != 3 || again.UpdatedCount != 0 {
		t.Fatalf("expected all unchanged, got %+v", again)
	}
	if len(agent.pushes) != pushesBefore {
		t.Fatalf("unchanged patch must not push, pushes=%v", agent.pushes)
	}
	if countVersions(t, database, tpl.ID) != 1 {
		// 种子数据没有版本行，因此只有本次修补产生的一条。
		// Seed rows had no version rows, so only the one created by the first patch exists.
		t.Fatalf("expected exactly one template version row")
	}
}

func TestPatchMemberListPushFailureIsPartialAndRetryable(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	seedHybridMemberConfigs(t, database, 1, 2)
	agent.offline[2] = true
	members := []string{"10.66.0.1:5801", "10.66.0.2:5801"}

	result, err := service.PatchMemberList(ctx, 1, "hybrid", members, "scale:1", 0)
	if err != nil {
		t.Fatalf("PatchMemberList: %v", err)
	}
	if !result.Partial() || result.PendingCount != 1 {
		t.Fatalf("expected one pending push, got %+v", result)
	}
	node2 := mustNodeConfig(t, database, 1, 2, ConfigTypeHazelcast)
	if node2.Version != 1 || node2.Content.String() != hazelcastWithMembers("localhost") || !node2.HasPendingPush() {
		t.Fatalf("offline host should keep its real content and park the patch as pending: v%d pending=%v", node2.Version, node2.HasPendingPush())
	}
	if node2.PendingContent.String() != hazelcastWithMembers(members...) {
		t.Fatalf("pending content = %q", node2.PendingContent.String())
	}

	// Agent 重连后通过现有待推送重试机制完成。
	// After the Agent reconnects the existing pending retry finishes it.
	agent.offline[2] = false
	if committed, err := service.RetryPendingPushes(ctx, 2); err != nil || committed != 1 {
		t.Fatalf("RetryPendingPushes = %d, %v", committed, err)
	}
	if got := mustNodeConfig(t, database, 1, 2, ConfigTypeHazelcast); got.Content.String() != hazelcastWithMembers(members...) || got.HasPendingPush() {
		t.Fatalf("retry did not commit the patch")
	}
}

func TestPatchMemberListSeparatedPatchesMasterWorkerAndHazelcast(t *testing.T) {
	service, database, _ := newScalingTestService(t)
	ctx := context.Background()
	original := hazelcastWithMembers("localhost")
	for _, configType := range []ConfigType{ConfigTypeHazelcast, ConfigTypeHazelcastMaster, ConfigTypeHazelcastWorker} {
		if err := database.Create(&Config{ClusterID: 1, ConfigType: configType, FilePath: GetConfigFilePath(configType), Content: db.ScriptText(original), Version: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.PatchMemberList(ctx, 1, "separated", []string{"10.0.0.1:5801", "10.0.0.2:5802"}, "", 0)
	if err != nil {
		t.Fatalf("PatchMemberList: %v", err)
	}
	if result.UpdatedCount != 3 {
		t.Fatalf("expected 3 templates patched, got %+v", result)
	}
}

func TestPatchMemberListReportsMissingMemberListPerConfig(t *testing.T) {
	service, database, _ := newScalingTestService(t)
	ctx := context.Background()
	if err := database.Create(&Config{ClusterID: 1, ConfigType: ConfigTypeHazelcast, FilePath: GetConfigFilePath(ConfigTypeHazelcast), Content: "hazelcast:\n  network:\n    port:\n      port: 5801\n", Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := service.PatchMemberList(ctx, 1, "hybrid", []string{"a:5801"}, "", 0)
	if err != nil {
		t.Fatalf("PatchMemberList: %v", err)
	}
	if result.FailedCount != 1 || !result.Partial() || result.Items[0].Message == "" {
		t.Fatalf("expected a failed item with a message, got %+v", result.Items)
	}
	if _, err := service.PatchMemberList(ctx, 1, "hybrid", nil, "", 0); !errors.Is(err, ErrEmptyMemberList) {
		t.Fatalf("expected ErrEmptyMemberList, got %v", err)
	}
}

func TestApplyTemplatesToHostRendersTemplatesWithMembers(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	templateHazelcast := hazelcastWithMembers("10.66.0.2:5801")
	jvmTemplate := "-Xms1g\n-Xmx1g\n"
	for _, record := range []*Config{
		{ClusterID: 1, ConfigType: ConfigTypeHazelcast, FilePath: GetConfigFilePath(ConfigTypeHazelcast), Content: db.ScriptText(templateHazelcast), Version: 3},
		{ClusterID: 1, ConfigType: ConfigTypeLog4j2, FilePath: GetConfigFilePath(ConfigTypeLog4j2), Content: scalingLog4j2PerJob, Version: 4},
		{ClusterID: 1, ConfigType: ConfigTypeJVMOptions, FilePath: GetConfigFilePath(ConfigTypeJVMOptions), Content: db.ScriptText(jvmTemplate), Version: 2},
		{ClusterID: 1, ConfigType: ConfigTypeHazelcastMaster, FilePath: GetConfigFilePath(ConfigTypeHazelcastMaster), Content: db.ScriptText(templateHazelcast), Version: 1},
	} {
		if err := database.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 新节点磁盘上是安装器写出的默认 hazelcast.yaml 与 log4j2，没有 jvm_options。
	// The new node has installer defaults for hazelcast.yaml and log4j2 but no jvm_options.
	agent.setFile(2, ConfigTypeHazelcast, hazelcastWithMembers("10.66.0.1:5801"))
	agent.setFile(2, ConfigTypeLog4j2, scalingLog4j2Default)
	members := []string{"10.66.0.1:5801", "10.66.0.2:5801"}

	result, err := service.ApplyTemplatesToHost(ctx, 1, 2, "hybrid", members, "scale:7", 5)
	if err != nil {
		t.Fatalf("ApplyTemplatesToHost: %v", err)
	}
	if result.Partial() || len(result.Items) != 4 {
		t.Fatalf("unexpected result: %+v", result.Items)
	}
	// 混合模式下未使用的 hazelcast-master.yaml 按模板原样下发，不替换成员列表。
	// In hybrid mode the unused hazelcast-master.yaml is applied verbatim without replacing members.
	if agent.files[2][ConfigTypeHazelcastMaster] != templateHazelcast {
		t.Fatalf("hazelcast-master.yaml should be the raw template in hybrid mode, got %q", agent.files[2][ConfigTypeHazelcastMaster])
	}

	wantHazelcast := hazelcastWithMembers(members...)
	if agent.files[2][ConfigTypeHazelcast] != wantHazelcast {
		t.Fatalf("node hazelcast.yaml = %q", agent.files[2][ConfigTypeHazelcast])
	}
	if agent.files[2][ConfigTypeLog4j2] != scalingLog4j2PerJob {
		t.Fatalf("node log4j2 should follow the template, got %q", agent.files[2][ConfigTypeLog4j2])
	}
	if agent.files[2][ConfigTypeJVMOptions] != jvmTemplate {
		t.Fatalf("node jvm_options should be pushed from the template, got %q", agent.files[2][ConfigTypeJVMOptions])
	}

	// 已有节点文件：v1=节点原值，v2=模板；节点缺失文件：直接推送后建立 v1。
	// Existing node file: v1=node value, v2=template; missing node file: pushed then created as v1.
	hz := mustNodeConfig(t, database, 1, 2, ConfigTypeHazelcast)
	if hz.Version != 2 || hz.Content.String() != wantHazelcast || countVersions(t, database, hz.ID) != 2 {
		t.Fatalf("hazelcast host config = v%d", hz.Version)
	}
	jvm := mustNodeConfig(t, database, 1, 2, ConfigTypeJVMOptions)
	if jvm.Version != 1 || jvm.Content.String() != jvmTemplate {
		t.Fatalf("jvm host config = v%d %q", jvm.Version, jvm.Content.String())
	}

	// 模板保持原样（成员列表只用于主机配置渲染）。
	// Templates stay untouched (members only render the host configs).
	if tpl := mustTemplate(t, database, 1, ConfigTypeHazelcast); tpl.Version != 3 || tpl.Content.String() != templateHazelcast {
		t.Fatalf("template changed: v%d", tpl.Version)
	}

	// 幂等：再次应用全部 unchanged。
	// Idempotent: applying again reports everything unchanged.
	again, err := service.ApplyTemplatesToHost(ctx, 1, 2, "hybrid", members, "scale:7", 5)
	if err != nil {
		t.Fatalf("second ApplyTemplatesToHost: %v", err)
	}
	if again.UnchangedCount != 4 {
		t.Fatalf("expected all unchanged on re-run, got %+v", again.Items)
	}
}

func TestApplyTemplatesToHostOfflineNodeFailsWithoutRecord(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	if err := database.Create(&Config{ClusterID: 1, ConfigType: ConfigTypeJVMOptions, FilePath: GetConfigFilePath(ConfigTypeJVMOptions), Content: "-Xmx1g\n", Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	agent.offline[2] = true

	result, err := service.ApplyTemplatesToHost(ctx, 1, 2, "hybrid", nil, "", 0)
	if err != nil {
		t.Fatalf("ApplyTemplatesToHost: %v", err)
	}
	if result.FailedCount != 1 || !result.Partial() {
		t.Fatalf("expected one failed item, got %+v", result.Items)
	}
	var count int64
	database.Model(&Config{}).Where("host_id = ?", 2).Count(&count)
	if count != 0 {
		t.Fatal("no host record should be created when the node file could not be written")
	}
}

func TestDeleteHostConfigsRemovesOnlyThatHostAndVersions(t *testing.T) {
	service, database, agent := newScalingTestService(t)
	ctx := context.Background()
	for _, hostID := range []uint{11, 12} {
		agent.setFile(hostID, ConfigTypeHazelcast, hazelcastWithMembers("10.0.0.11:5801", "10.0.0.12:5801"))
		agent.setFile(hostID, ConfigTypeLog4j2, scalingLog4j2PerJob)
		if err := service.EnsureClusterConfigsForHost(ctx, 1, hostID, "/opt/seatunnel", 1); err != nil {
			t.Fatalf("EnsureClusterConfigsForHost(%d): %v", hostID, err)
		}
	}
	removed := mustNodeConfig(t, database, 1, 12, ConfigTypeHazelcast)
	template := mustTemplate(t, database, 1, ConfigTypeHazelcast)

	deleted, err := service.DeleteHostConfigs(ctx, 1, 12)
	if err != nil {
		t.Fatalf("DeleteHostConfigs: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 host records deleted, got %d", deleted)
	}
	var remaining int64
	database.Model(&Config{}).Where("cluster_id = ? AND host_id = ?", 1, 12).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("host 12 records should be gone, %d left", remaining)
	}
	if countVersions(t, database, removed.ID) != 0 {
		t.Fatal("versions of deleted host records must be removed too")
	}
	// 其他主机与模板保持不变。
	// Other hosts and templates stay untouched.
	mustNodeConfig(t, database, 1, 11, ConfigTypeHazelcast)
	if after := mustTemplate(t, database, 1, ConfigTypeHazelcast); after.Version != template.Version || after.Content != template.Content {
		t.Fatal("template must not change when deleting host configs")
	}

	again, err := service.DeleteHostConfigs(ctx, 1, 12)
	if err != nil || again != 0 {
		t.Fatalf("second delete should be a no-op, got %d %v", again, err)
	}
}
