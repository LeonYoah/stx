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

package installer

import "testing"

func TestIsSTXJavaProxyCommandLine(t *testing.T) {
	t.Parallel()

	proxyLine := `/usr/bin/java -DSEATUNNEL_HOME=/opt/seatunnel-3.0.0 -cp /opt/seatunnel-3.0.0/lib/*:/Users/mac/.stx/agent/lib/stx-java-proxy-v3.jar io.github.leonyoah.stx.proxy.StxJavaProxyApplication`
	if !isSTXJavaProxyCommandLine(proxyLine) {
		t.Fatal("expected proxy command line to match")
	}
	if isSTXJavaProxyCommandLine("java -cp seatunnel-starter.jar org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer") {
		t.Fatal("seatunnel server must not match proxy command line")
	}
	if isSTXJavaProxyCommandLine("") {
		t.Fatal("empty command line must not match")
	}
}

func TestSTXJavaProxyCommandLineMatchesDesired(t *testing.T) {
	t.Parallel()

	oldLine := `/usr/bin/java -DSEATUNNEL_HOME=/Users/mac/opt/seatunnel-2.3.13 -Dstx.java.proxy.seatunnel.home=/Users/mac/opt/seatunnel-2.3.13 -cp /Users/mac/opt/seatunnel-2.3.13/lib/*:/Users/mac/.stx/agent/lib/stx-java-proxy-v2.jar io.github.leonyoah.stx.proxy.StxJavaProxyApplication`
	newLine := `/usr/bin/java -DSEATUNNEL_HOME=/Users/mac/opt/seatunnel-3.0.0 -Dstx.java.proxy.seatunnel.home=/Users/mac/opt/seatunnel-3.0.0 -cp /Users/mac/opt/seatunnel-3.0.0/lib/*:/Users/mac/.stx/agent/lib/stx-java-proxy-v3.jar io.github.leonyoah.stx.proxy.StxJavaProxyApplication`

	if stxJavaProxyCommandLineMatchesDesired(oldLine, "/Users/mac/opt/seatunnel-3.0.0", "3.0.0") {
		t.Fatal("old 2.3.13/v2 process must not match desired 3.0.0/v3 runtime")
	}
	if !stxJavaProxyCommandLineMatchesDesired(newLine, "/Users/mac/opt/seatunnel-3.0.0", "3.0.0") {
		t.Fatal("expected 3.0.0/v3 process to match desired runtime")
	}
	if stxJavaProxyCommandLineMatchesDesired(newLine, "/Users/mac/opt/seatunnel-3.0.0", "2.3.13") {
		t.Fatal("v3 jar must not match desired v2 epoch")
	}
}
