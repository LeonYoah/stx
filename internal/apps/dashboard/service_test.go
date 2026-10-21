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

package dashboard

import (
	"testing"

	"github.com/LeonYoah/stx/internal/apps/cluster"
)

func TestCountNodeRoleCountsHybridNodesAsBoth(t *testing.T) {
	s := &ClusterSummary{}
	for _, role := range []cluster.NodeRole{cluster.NodeRoleMasterWorker, cluster.NodeRoleMasterWorker, cluster.NodeRoleMaster, cluster.NodeRoleWorker} {
		countNodeRole(s, role)
	}
	if s.MasterNodes != 3 || s.WorkerNodes != 3 {
		t.Fatalf("master=%d worker=%d, want 3/3", s.MasterNodes, s.WorkerNodes)
	}
}
