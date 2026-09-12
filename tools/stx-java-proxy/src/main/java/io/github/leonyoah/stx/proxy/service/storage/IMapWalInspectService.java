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

package io.github.leonyoah.stx.proxy.service.storage;

import org.apache.seatunnel.shade.com.fasterxml.jackson.core.type.TypeReference;
import org.apache.seatunnel.shade.org.apache.commons.lang3.ClassUtils;

import org.apache.seatunnel.engine.serializer.api.Serializer;
import org.apache.seatunnel.engine.serializer.protobuf.ProtoStuffSerializer;

import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.IOException;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public class IMapWalInspectService {

    private static final String IMAP_FILE_DATA_CLASS =
            "org.apache.seatunnel.engine.imap.storage.file.bean.IMapFileData";
    private static final int WAL_DATA_METADATA_LENGTH = 12;

    private final RuntimeStoragePreviewService previewService = new RuntimeStoragePreviewService();
    private final Serializer serializer = new ProtoStuffSerializer();

    public Map<String, Object> inspect(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "IMAP WAL inspect",
                "Parse IMAP WAL records and deserialize key/value entries via SeaTunnel runtime serializer.",
                () -> doInspect(request));
    }

    private Map<String, Object> doInspect(Map<String, Object> request) throws IOException {
        Map<String, Object> preview = previewService.preview(request);
        byte[] rawBytes = loadRawBytes(request, preview);

        List<Map<String, Object>> entries = new ArrayList<>();
        int offset = 0;
        while (offset + WAL_DATA_METADATA_LENGTH <= rawBytes.length) {
            int recordLength = byteArrayToInt(rawBytes, offset);
            offset += WAL_DATA_METADATA_LENGTH;
            if (recordLength <= 0 || offset + recordLength > rawBytes.length) {
                break;
            }
            byte[] recordBytes = new byte[recordLength];
            System.arraycopy(rawBytes, offset, recordBytes, 0, recordLength);
            offset += recordLength;
            entries.add(buildEntry(deserializeIMapFileData(recordBytes), recordLength));
        }

        Map<String, Object> response = new LinkedHashMap<>(preview);
        response.put("entryCount", entries.size());
        response.put("entries", entries);
        return response;
    }

    private byte[] loadRawBytes(Map<String, Object> request, Map<String, Object> preview)
            throws IOException {
        Object inlineContent = request.get("contentBase64");
        if (inlineContent instanceof String && !((String) inlineContent).trim().isEmpty()) {
            return previewService.loadRawBytes(request);
        }
        Map<String, Object> bounded = new LinkedHashMap<>(request);
        Object sizeBytes = preview.get("sizeBytes");
        if (sizeBytes instanceof Number) {
            bounded.put("maxBytes", ((Number) sizeBytes).longValue());
        }
        return previewService.loadRawBytes(bounded);
    }

    private Object deserializeIMapFileData(byte[] recordBytes) {
        try {
            Class<?> clazz = Class.forName(IMAP_FILE_DATA_CLASS);
            return serializer.deserialize(recordBytes, clazz);
        } catch (ClassNotFoundException e) {
            throw new ProxyException(500, "IMapFileData class is unavailable", e);
        } catch (IOException e) {
            throw new ProxyException(500, "Failed to deserialize IMAP WAL record", e);
        }
    }

    private Map<String, Object> buildEntry(Object record, int recordLength) {
        Map<String, Object> item = new LinkedHashMap<>();
        item.put("deleted", invokeBoolean(record, "isDeleted"));
        item.put("timestamp", invoke(record, "getTimestamp"));
        item.put("recordBytes", recordLength);
        String keyClassName = asString(invoke(record, "getKeyClassName"));
        String valueClassName = asString(invoke(record, "getValueClassName"));
        item.put("keyClassName", keyClassName);
        item.put("valueClassName", valueClassName);
        item.put("key", deserializeValue((byte[]) invoke(record, "getKey"), keyClassName));
        item.put("value", deserializeValue((byte[]) invoke(record, "getValue"), valueClassName));
        return item;
    }

    private Object deserializeValue(byte[] bytes, String className) {
        if (bytes == null) {
            return null;
        }
        if (className == null || className.trim().isEmpty()) {
            return fallbackValue(bytes);
        }
        try {
            Class<?> clazz = ClassUtils.getClass(className.trim());
            Object value = serializer.deserialize(bytes, clazz);
            return normalizeValue(value);
        } catch (Exception e) {
            Map<String, Object> fallback = new LinkedHashMap<>();
            fallback.put("className", className);
            fallback.put("error", e.getMessage());
            fallback.put("preview", fallbackValue(bytes));
            return fallback;
        }
    }

    private Object normalizeValue(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof String
                || value instanceof Number
                || value instanceof Boolean
                || value instanceof Map
                || value instanceof List) {
            return value;
        }
        try {
            String json = org.apache.seatunnel.common.utils.JsonUtils.toJsonString(value);
            return org.apache.seatunnel.common.utils.JsonUtils.parseObject(
                    json, new TypeReference<Object>() {});
        } catch (Exception ignored) {
            return String.valueOf(value);
        }
    }

    private Object fallbackValue(byte[] bytes) {
        Map<String, Object> fallback = new LinkedHashMap<>();
        fallback.put("sizeBytes", bytes.length);
        fallback.put("hexPreview", toHex(bytes, 128));
        return fallback;
    }

    private Object invoke(Object target, String methodName) {
        if (target == null) {
            return null;
        }
        try {
            return target.getClass().getMethod(methodName).invoke(target);
        } catch (Exception e) {
            throw new ProxyException(500, "Failed to inspect IMAP WAL record via " + methodName, e);
        }
    }

    private boolean invokeBoolean(Object target, String methodName) {
        Object value = invoke(target, methodName);
        return value instanceof Boolean && (Boolean) value;
    }

    private String asString(Object value) {
        return value == null ? null : String.valueOf(value);
    }

    private int byteArrayToInt(byte[] bytes, int offset) {
        if (offset + 3 >= bytes.length) {
            return 0;
        }
        return ((bytes[offset + 3] & 0xFF) << 24)
                | ((bytes[offset + 2] & 0xFF) << 16)
                | ((bytes[offset + 1] & 0xFF) << 8)
                | (bytes[offset] & 0xFF);
    }

    private String toHex(byte[] bytes, int limit) {
        int size = Math.min(bytes.length, limit);
        StringBuilder builder = new StringBuilder(size * 3);
        for (int i = 0; i < size; i++) {
            if (i > 0) {
                builder.append(' ');
            }
            builder.append(String.format("%02x", bytes[i] & 0xff));
        }
        if (bytes.length > limit) {
            builder.append(" …");
        }
        return builder.toString();
    }
}
