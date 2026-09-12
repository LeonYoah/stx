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

package io.github.leonyoah.stx.proxy;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Arrays;
import java.util.concurrent.CountDownLatch;

public class StxJavaProxyApplication {

    private static final Logger LOG = LoggerFactory.getLogger(StxJavaProxyApplication.class);

    private StxJavaProxyApplication() {}

    public static void main(String[] args) throws Exception {
        if (args.length > 0 && "probe-once".equalsIgnoreCase(args[0])) {
            int exitCode =
                    new StxJavaProxyCli()
                            .run(Arrays.copyOfRange(args, 1, args.length), System.out, System.err);
            if (exitCode != 0) {
                System.exit(exitCode);
            }
            return;
        }
        int port = Integer.parseInt(System.getProperty("stx.java.proxy.port", "18080"));
        int workers =
                Integer.parseInt(
                        System.getProperty(
                                "stx.java.proxy.workerThreads",
                                String.valueOf(
                                        Math.max(4, Runtime.getRuntime().availableProcessors()))));
        StxJavaProxyServer server = new StxJavaProxyServer(port, workers);
        CountDownLatch keepAlive = new CountDownLatch(1);
        Runtime.getRuntime()
                .addShutdownHook(
                        new Thread(
                                () -> {
                                    server.stop(0);
                                    keepAlive.countDown();
                                },
                                "stx-java-proxy-shutdown"));
        server.start();
        LOG.info("SeaTunnel stx-java-proxy started on port {}", port);
        keepAlive.await();
    }
}
