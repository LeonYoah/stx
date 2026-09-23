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

import org.apache.seatunnel.shade.com.fasterxml.jackson.core.type.TypeReference;

import org.apache.seatunnel.common.utils.JsonUtils;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import io.github.leonyoah.stx.proxy.service.catalog.CatalogProbeService;
import io.github.leonyoah.stx.proxy.service.config.ConfigResourceService;
import io.github.leonyoah.stx.proxy.service.config.ConfigValidationService;
import io.github.leonyoah.stx.proxy.service.config.PreviewConfigService;
import io.github.leonyoah.stx.proxy.service.config.SinkSaveModePreviewService;
import io.github.leonyoah.stx.proxy.service.plugin.PluginEnumValueService;
import io.github.leonyoah.stx.proxy.service.plugin.PluginOptionSchemaService;
import io.github.leonyoah.stx.proxy.service.plugin.PluginRuntimeService;
import io.github.leonyoah.stx.proxy.service.plugin.TemplateRenderService;
import io.github.leonyoah.stx.proxy.service.storage.CheckpointDeserializeService;
import io.github.leonyoah.stx.proxy.service.storage.CheckpointProbeService;
import io.github.leonyoah.stx.proxy.service.storage.CheckpointSourceStateInspectService;
import io.github.leonyoah.stx.proxy.service.storage.IMapProbeService;
import io.github.leonyoah.stx.proxy.service.storage.IMapWalInspectService;
import io.github.leonyoah.stx.proxy.service.storage.RuntimeStorageListService;
import io.github.leonyoah.stx.proxy.service.storage.RuntimeStoragePreviewService;
import io.github.leonyoah.stx.proxy.service.storage.RuntimeStorageStatService;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class StxJavaProxyServer {

    private static final Logger LOG = LoggerFactory.getLogger(StxJavaProxyServer.class);

    private final HttpServer httpServer;
    private final ExecutorService executorService;
    private final ConfigResourceService configResourceService;
    private final ConfigValidationService configValidationService;
    private final CatalogProbeService catalogProbeService;
    private final CheckpointProbeService checkpointProbeService;
    private final IMapProbeService iMapProbeService;
    private final RuntimeStorageStatService runtimeStorageStatService;
    private final RuntimeStorageListService runtimeStorageListService;
    private final RuntimeStoragePreviewService runtimeStoragePreviewService;
    private final CheckpointDeserializeService checkpointDeserializeService;
    private final CheckpointSourceStateInspectService checkpointSourceStateInspectService;
    private final IMapWalInspectService iMapWalInspectService;
    private final PreviewConfigService previewConfigService;
    private final PluginRuntimeService pluginRuntimeService;
    private final PluginOptionSchemaService pluginOptionSchemaService;
    private final TemplateRenderService templateRenderService;
    private final PluginEnumValueService pluginEnumValueService;
    private final SinkSaveModePreviewService sinkSaveModePreviewService;

    public StxJavaProxyServer(int port, int workerThreads) throws IOException {
        this(
                HttpServer.create(new InetSocketAddress(port), 0),
                Executors.newFixedThreadPool(workerThreads),
                new ConfigResourceService(),
                new ConfigValidationService(),
                new CatalogProbeService(),
                new CheckpointProbeService(),
                new IMapProbeService(),
                new RuntimeStorageStatService(),
                new RuntimeStorageListService(),
                new RuntimeStoragePreviewService(),
                new CheckpointDeserializeService(),
                new CheckpointSourceStateInspectService(),
                new IMapWalInspectService(),
                new PreviewConfigService(),
                new PluginRuntimeService(),
                new PluginOptionSchemaService(),
                new TemplateRenderService(),
                new PluginEnumValueService(),
                new SinkSaveModePreviewService());
    }

    StxJavaProxyServer(
            HttpServer httpServer,
            ExecutorService executorService,
            ConfigResourceService configResourceService,
            ConfigValidationService configValidationService,
            CatalogProbeService catalogProbeService,
            CheckpointProbeService checkpointProbeService,
            IMapProbeService iMapProbeService,
            RuntimeStorageStatService runtimeStorageStatService,
            RuntimeStorageListService runtimeStorageListService,
            RuntimeStoragePreviewService runtimeStoragePreviewService,
            CheckpointDeserializeService checkpointDeserializeService,
            CheckpointSourceStateInspectService checkpointSourceStateInspectService,
            IMapWalInspectService iMapWalInspectService,
            PreviewConfigService previewConfigService,
            PluginRuntimeService pluginRuntimeService,
            PluginOptionSchemaService pluginOptionSchemaService,
            TemplateRenderService templateRenderService,
            PluginEnumValueService pluginEnumValueService,
            SinkSaveModePreviewService sinkSaveModePreviewService) {
        this.httpServer = httpServer;
        this.executorService = executorService;
        this.configResourceService = configResourceService;
        this.configValidationService = configValidationService;
        this.catalogProbeService = catalogProbeService;
        this.checkpointProbeService = checkpointProbeService;
        this.iMapProbeService = iMapProbeService;
        this.runtimeStorageStatService = runtimeStorageStatService;
        this.runtimeStorageListService = runtimeStorageListService;
        this.runtimeStoragePreviewService = runtimeStoragePreviewService;
        this.checkpointDeserializeService = checkpointDeserializeService;
        this.checkpointSourceStateInspectService = checkpointSourceStateInspectService;
        this.iMapWalInspectService = iMapWalInspectService;
        this.previewConfigService = previewConfigService;
        this.pluginRuntimeService = pluginRuntimeService;
        this.pluginOptionSchemaService = pluginOptionSchemaService;
        this.templateRenderService = templateRenderService;
        this.pluginEnumValueService = pluginEnumValueService;
        this.sinkSaveModePreviewService = sinkSaveModePreviewService;
        registerContexts();
        this.httpServer.setExecutor(executorService);
    }

    public void start() {
        httpServer.start();
    }

    public void stop(int delaySeconds) {
        httpServer.stop(delaySeconds);
        executorService.shutdownNow();
    }

    private void registerContexts() {
        httpServer.createContext(
                "/healthz",
                exchange -> {
                    Runtime runtime = Runtime.getRuntime();
                    Map<String, Object> resp = new LinkedHashMap<>();
                    resp.put("ok", true);
                    Map<String, Object> memory = new LinkedHashMap<>();
                    memory.put("maxMemoryBytes", runtime.maxMemory());
                    memory.put("totalMemoryBytes", runtime.totalMemory());
                    memory.put("freeMemoryBytes", runtime.freeMemory());
                    memory.put("usedMemoryBytes", runtime.totalMemory() - runtime.freeMemory());
                    memory.put("maxMemoryMb", runtime.maxMemory() / (1024 * 1024));
                    resp.put("jvmMemory", memory);
                    writeJson(exchange, 200, resp);
                });
        httpServer.createContext(
                "/api/v1/config/validate",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return configValidationService.validate(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/config/dag",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return configResourceService.inspectDag(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/config/webui-dag",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return configResourceService.inspectWebUiDag(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/config/preview/source",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return previewConfigService.deriveSourcePreview(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/config/preview/transform",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return previewConfigService.deriveTransformPreview(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/config/preview/sink-savemode",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return sinkSaveModePreviewService.preview(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/catalog/probe",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return catalogProbeService.probe(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/plugin/list",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return pluginRuntimeService.list(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/plugin/options",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return pluginOptionSchemaService.inspect(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/plugin/template",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return templateRenderService.render(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/plugin/enum-values",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return pluginEnumValueService.listValues(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/checkpoint/probe",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return checkpointProbeService.probe(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/imap/probe",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return iMapProbeService.probe(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/checkpoint/stat",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStorageStatService.stat(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/imap/stat",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStorageStatService.stat(request);
                    }
                });

        httpServer.createContext(
                "/api/v1/storage/checkpoint/list",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStorageListService.list(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/imap/list",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStorageListService.list(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/checkpoint/preview",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStoragePreviewService.preview(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/imap/preview",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return runtimeStoragePreviewService.preview(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/checkpoint/inspect",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return checkpointDeserializeService.inspect(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/checkpoint/inspect-source-state",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return checkpointSourceStateInspectService.inspect(request);
                    }
                });
        httpServer.createContext(
                "/api/v1/storage/imap/inspect-wal",
                new JsonPostHandler() {
                    @Override
                    protected Object handleRequest(Map<String, Object> request) {
                        return iMapWalInspectService.inspect(request);
                    }
                });
    }

    private abstract class JsonPostHandler implements HttpHandler {
        @Override
        public void handle(HttpExchange exchange) throws IOException {
            try {
                if (!"POST".equalsIgnoreCase(exchange.getRequestMethod())) {
                    writeJson(exchange, 405, errorBody("Method not allowed"));
                    return;
                }
                Map<String, Object> request = parseRequest(exchange.getRequestBody());
                Object result = handleRequest(request);
                writeJson(exchange, 200, result);
            } catch (ProxyException e) {
                if (e.getCause() != null) {
                    LOG.warn("Request failed: {}", e.getMessage(), e);
                } else {
                    LOG.warn("Request failed: {}", e.getMessage());
                }
                writeJson(exchange, e.getStatusCode(), errorBody(e.getMessage()));
            } catch (Exception e) {
                LOG.error("Unexpected request failure", e);
                writeJson(exchange, 500, errorBody(e.getMessage()));
            } catch (Throwable e) {
                LOG.error("Fatal request failure", e);
                writeJson(exchange, 500, errorBody(e.getMessage()));
            }
        }

        protected abstract Object handleRequest(Map<String, Object> request);
    }

    private Map<String, Object> parseRequest(InputStream requestBody) throws IOException {
        byte[] bytes = readAllBytes(requestBody);
        if (bytes.length == 0) {
            return Collections.emptyMap();
        }
        return JsonUtils.parseObject(
                new String(bytes, StandardCharsets.UTF_8),
                new TypeReference<Map<String, Object>>() {});
    }

    private byte[] readAllBytes(InputStream inputStream) throws IOException {
        byte[] buffer = new byte[4096];
        int bytesRead;
        try (InputStream in = inputStream;
                java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream()) {
            while ((bytesRead = in.read(buffer)) >= 0) {
                out.write(buffer, 0, bytesRead);
            }
            return out.toByteArray();
        }
    }

    private void writeJson(HttpExchange exchange, int statusCode, Object body) throws IOException {
        byte[] payload = JsonUtils.toJsonString(body).getBytes(StandardCharsets.UTF_8);
        Headers headers = exchange.getResponseHeaders();
        headers.add("Content-Type", "application/json; charset=utf-8");
        exchange.sendResponseHeaders(statusCode, payload.length);
        try (OutputStream outputStream = exchange.getResponseBody()) {
            outputStream.write(payload);
        }
    }

    private Map<String, Object> errorBody(String message) {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("ok", false);
        body.put("message", message);
        return body;
    }
}
