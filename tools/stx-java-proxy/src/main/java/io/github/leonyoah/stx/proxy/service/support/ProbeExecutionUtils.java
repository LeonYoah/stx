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

package io.github.leonyoah.stx.proxy.service.support;

import java.util.Map;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.SynchronousQueue;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicInteger;

public final class ProbeExecutionUtils {

    public static final long DEFAULT_PROBE_TIMEOUT_MS = 15_000L;

    private static final long MIN_PROBE_TIMEOUT_MS = 500L;

    private static final ThreadPoolExecutor EXECUTOR =
            new ThreadPoolExecutor(
                    0,
                    Integer.MAX_VALUE,
                    60L,
                    TimeUnit.SECONDS,
                    new SynchronousQueue<>(),
                    new ProbeThreadFactory());

    private ProbeExecutionUtils() {}

    public static long resolveProbeTimeoutMs(Map<String, Object> request) {
        long timeoutMs = ProxyRequestUtils.getLong(request, "probeTimeoutMs", -1L);
        if (timeoutMs < 0) {
            timeoutMs = ProxyRequestUtils.getLong(request, "timeoutMs", DEFAULT_PROBE_TIMEOUT_MS);
        }
        if (timeoutMs < MIN_PROBE_TIMEOUT_MS) {
            throw new ProxyException(
                    400,
                    "Probe timeout is too small: "
                            + timeoutMs
                            + " ms. Use probeTimeoutMs >= "
                            + MIN_PROBE_TIMEOUT_MS
                            + " ms");
        }
        return timeoutMs;
    }

    public static <T> T runWithTimeout(
            Map<String, Object> request,
            String operationName,
            String timeoutHint,
            Callable<T> callable) {
        long timeoutMs = resolveProbeTimeoutMs(request);
        Future<T> future = EXECUTOR.submit(callable);
        try {
            return future.get(timeoutMs, TimeUnit.MILLISECONDS);
        } catch (TimeoutException e) {
            future.cancel(true);
            throw new ProxyException(
                    504,
                    operationName + " timed out after " + timeoutMs + " ms. " + timeoutHint,
                    e);
        } catch (InterruptedException e) {
            future.cancel(true);
            Thread.currentThread().interrupt();
            throw new ProxyException(500, operationName + " was interrupted", e);
        } catch (ExecutionException e) {
            Throwable cause = e.getCause();
            if (cause instanceof ProxyException) {
                throw (ProxyException) cause;
            }
            String message = cause == null ? e.getMessage() : cause.getMessage();
            throw new ProxyException(500, operationName + " failed: " + message, cause);
        }
    }

    private static final class ProbeThreadFactory implements ThreadFactory {

        private final AtomicInteger counter = new AtomicInteger();

        @Override
        public Thread newThread(Runnable runnable) {
            Thread thread = Executors.defaultThreadFactory().newThread(runnable);
            thread.setName("stx-java-proxy-probe-" + counter.incrementAndGet());
            thread.setDaemon(true);
            return thread;
        }
    }
}
