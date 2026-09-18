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

import io.github.leonyoah.stx.proxy.service.storage.CheckpointProbeService;
import io.github.leonyoah.stx.proxy.service.storage.IMapProbeService;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.LinkedHashMap;
import java.util.Map;

public class StxJavaProxyCli {

    private final ProbeInvoker checkpointProbeInvoker;
    private final ProbeInvoker iMapProbeInvoker;

    public StxJavaProxyCli() {
        this(new CheckpointProbeService()::probe, new IMapProbeService()::probe);
    }

    StxJavaProxyCli(ProbeInvoker checkpointProbeInvoker, ProbeInvoker iMapProbeInvoker) {
        this.checkpointProbeInvoker = checkpointProbeInvoker;
        this.iMapProbeInvoker = iMapProbeInvoker;
    }

    public int run(String[] args, OutputStream stdout, OutputStream stderr) throws IOException {
        PrintStream errStream = new PrintStream(stderr, false, StandardCharsets.UTF_8.name());
        try {
            CliArguments cliArguments = CliArguments.parse(args);
            Map<String, Object> request = loadRequest(cliArguments);
            Map<String, Object> response = invokeProbe(cliArguments.kind, request);
            writeJsonResponse(response, cliArguments.responseFile, stdout);
            return 0;
        } catch (ProxyException e) {
            writeJsonResponse(
                    errorBody(e.getStatusCode(), e.getMessage()), findResponseFile(args), stdout);
            errStream.println(e.getMessage());
            return 1;
        } catch (IllegalArgumentException e) {
            writeJsonResponse(errorBody(400, e.getMessage()), findResponseFile(args), stdout);
            errStream.println(e.getMessage());
            return 1;
        } catch (Exception e) {
            String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
            writeJsonResponse(errorBody(500, message), findResponseFile(args), stdout);
            errStream.println(message);
            return 1;
        } finally {
            errStream.flush();
        }
    }

    private Map<String, Object> invokeProbe(String kind, Map<String, Object> request) {
        if ("checkpoint".equalsIgnoreCase(kind)) {
            return checkpointProbeInvoker.probe(request);
        }
        if ("imap".equalsIgnoreCase(kind)) {
            return iMapProbeInvoker.probe(request);
        }
        throw new IllegalArgumentException("Unsupported probe kind: " + kind);
    }

    private Map<String, Object> loadRequest(CliArguments cliArguments) throws IOException {
        if (cliArguments.requestJson != null) {
            return JsonUtils.parseObject(
                    cliArguments.requestJson, new TypeReference<Map<String, Object>>() {});
        }
        if (cliArguments.requestFile == null) {
            throw new IllegalArgumentException(
                    "Missing request payload, use --request-file or --request-json");
        }
        byte[] bytes = Files.readAllBytes(cliArguments.requestFile);
        if (bytes.length == 0) {
            return new LinkedHashMap<>();
        }
        return JsonUtils.parseObject(
                new String(bytes, StandardCharsets.UTF_8),
                new TypeReference<Map<String, Object>>() {});
    }

    private void writeJsonResponse(Map<String, Object> body, Path responseFile, OutputStream stdout)
            throws IOException {
        byte[] payload = JsonUtils.toJsonString(body).getBytes(StandardCharsets.UTF_8);
        if (responseFile != null) {
            Path parent = responseFile.getParent();
            if (parent != null) {
                Files.createDirectories(parent);
            }
            Files.write(responseFile, payload);
            return;
        }
        stdout.write(payload);
        stdout.write('\n');
        stdout.flush();
    }

    private Map<String, Object> errorBody(int statusCode, String message) {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("ok", false);
        body.put("statusCode", statusCode);
        body.put("message", message);
        return body;
    }

    private Path findResponseFile(String[] args) {
        for (int i = 0; i < args.length; i++) {
            if ("--response-file".equals(args[i]) && i + 1 < args.length) {
                return Paths.get(args[i + 1]);
            }
            if (args[i].startsWith("--response-file=")) {
                return Paths.get(args[i].substring("--response-file=".length()));
            }
        }
        return null;
    }

    @FunctionalInterface
    interface ProbeInvoker {
        Map<String, Object> probe(Map<String, Object> request);
    }

    private static final class CliArguments {
        private final String kind;
        private final Path requestFile;
        private final String requestJson;
        private final Path responseFile;

        private CliArguments(String kind, Path requestFile, String requestJson, Path responseFile) {
            this.kind = kind;
            this.requestFile = requestFile;
            this.requestJson = requestJson;
            this.responseFile = responseFile;
        }

        private static CliArguments parse(String[] args) {
            if (args.length == 0) {
                throw new IllegalArgumentException(
                        "Missing probe kind, expected: checkpoint or imap");
            }
            String kind = args[0];
            Path requestFile = null;
            String requestJson = null;
            Path responseFile = null;
            for (int i = 1; i < args.length; i++) {
                String arg = args[i];
                if ("--request-file".equals(arg)) {
                    requestFile = Paths.get(requireValue("--request-file", args, ++i));
                    continue;
                }
                if (arg.startsWith("--request-file=")) {
                    requestFile = Paths.get(arg.substring("--request-file=".length()));
                    continue;
                }
                if ("--request-json".equals(arg)) {
                    requestJson = requireValue("--request-json", args, ++i);
                    continue;
                }
                if (arg.startsWith("--request-json=")) {
                    requestJson = arg.substring("--request-json=".length());
                    continue;
                }
                if ("--response-file".equals(arg)) {
                    responseFile = Paths.get(requireValue("--response-file", args, ++i));
                    continue;
                }
                if (arg.startsWith("--response-file=")) {
                    responseFile = Paths.get(arg.substring("--response-file=".length()));
                    continue;
                }
                throw new IllegalArgumentException("Unsupported CLI argument: " + arg);
            }
            if (requestFile != null && requestJson != null) {
                throw new IllegalArgumentException(
                        "Use either --request-file or --request-json, not both");
            }
            return new CliArguments(kind, requestFile, requestJson, responseFile);
        }

        private static String requireValue(String flag, String[] args, int index) {
            if (index >= args.length) {
                throw new IllegalArgumentException("Missing value for " + flag);
            }
            return args[index];
        }
    }
}
