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

package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureRegistryCredentialSecretCreatesOwnedSecretAndMapsDockerHub(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	name, err := EnsureRegistryCredentialSecret(context.Background(), clientset, "ns", "demo", "reg.example.com", "user", "pass")
	require.NoError(t, err)
	assert.Equal(t, "stx-regcred-demo", name)
	secret, err := clientset.CoreV1().Secrets("ns").Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
	assert.True(t, isOwnedByCluster(secret.Labels, "demo"))
	raw := string(secret.Data[corev1.DockerConfigJsonKey])
	assert.Contains(t, raw, "reg.example.com")
	assert.Contains(t, raw, base64.StdEncoding.EncodeToString([]byte("user:pass")))

	// docker.io 凭据写入必须映射到 kubelet 识别的 https://index.docker.io/v1/，并保留既有仓库凭据。
	// docker.io credentials must map to https://index.docker.io/v1/ and preserve existing entries.
	_, err = EnsureRegistryCredentialSecret(context.Background(), clientset, "ns", "demo", "docker.io", "hubuser", "hubpass")
	require.NoError(t, err)
	secret, err = clientset.CoreV1().Secrets("ns").Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	var decoded dockerConfigPayload
	require.NoError(t, json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &decoded))
	assert.Contains(t, decoded.Auths, "reg.example.com", "previous registry entry must be retained during rolling upgrade")
	assert.Contains(t, decoded.Auths, "https://index.docker.io/v1/")
	assert.Equal(t, "hubuser", decoded.Auths["https://index.docker.io/v1/"].Username)
}

func TestEnsureRegistryCredentialSecretUpdatesExisting(t *testing.T) {
	names := ResourceNamesFor("demo")
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.RegistryCredentialSecret,
			Namespace: "ns",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "stx",
				LabelClusterCodeKey:            "demo",
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{"old.example.com":{"username":"u1","password":"p1","auth":"dTE6cDE="}}}`)},
	}
	clientset := fake.NewSimpleClientset(existing)
	_, err := EnsureRegistryCredentialSecret(context.Background(), clientset, "ns", "demo", "reg2.example.com", "u2", "p2")
	require.NoError(t, err)
	secret, err := clientset.CoreV1().Secrets("ns").Get(context.Background(), names.RegistryCredentialSecret, metav1.GetOptions{})
	require.NoError(t, err)
	var decoded dockerConfigPayload
	require.NoError(t, json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &decoded))
	assert.Contains(t, decoded.Auths, "old.example.com")
	assert.Contains(t, decoded.Auths, "reg2.example.com")
}

func TestEnsureRegistryCredentialSecretRejectsForeignConflict(t *testing.T) {
	names := ResourceNamesFor("demo")
	foreign := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: names.RegistryCredentialSecret, Namespace: "ns"},
	}
	clientset := fake.NewSimpleClientset(foreign)
	_, err := EnsureRegistryCredentialSecret(context.Background(), clientset, "ns", "demo", "h", "u", "p")
	assert.Error(t, err)
}
