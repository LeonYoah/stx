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

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public final class ProxyRequestUtils {

    private ProxyRequestUtils() {}

    public static String getRequiredString(Map<String, Object> request, String key) {
        String value = getOptionalString(request, key);
        if (StringUtils.isBlank(value)) {
            throw new ProxyException(400, "Missing required field: " + key);
        }
        return value;
    }

    public static String getOptionalString(Map<String, Object> request, String key) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return null;
        }
        return String.valueOf(request.get(key));
    }

    public static boolean getBoolean(
            Map<String, Object> request, String key, boolean defaultValue) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return defaultValue;
        }
        Object value = request.get(key);
        if (value instanceof Boolean) {
            return (Boolean) value;
        }
        return Boolean.parseBoolean(String.valueOf(value));
    }

    public static long getLong(Map<String, Object> request, String key, long defaultValue) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return defaultValue;
        }
        Object value = request.get(key);
        if (value instanceof Number) {
            return ((Number) value).longValue();
        }
        return Long.parseLong(String.valueOf(value));
    }

    @SuppressWarnings("unchecked")
    public static Map<String, Object> getMap(Map<String, Object> request, String key) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return Collections.emptyMap();
        }
        Object value = request.get(key);
        if (!(value instanceof Map)) {
            throw new ProxyException(400, "Field '" + key + "' must be an object");
        }
        return new LinkedHashMap<>((Map<String, Object>) value);
    }

    @SuppressWarnings("unchecked")
    public static List<String> getStringList(Map<String, Object> request, String key) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return Collections.emptyList();
        }
        Object value = request.get(key);
        if (!(value instanceof List)) {
            throw new ProxyException(400, "Field '" + key + "' must be an array");
        }
        List<Object> rawList = (List<Object>) value;
        List<String> result = new ArrayList<>(rawList.size());
        for (Object item : rawList) {
            result.add(String.valueOf(item));
        }
        return result;
    }

    public static Map<String, String> toStringMap(Map<String, Object> source) {
        Map<String, String> result = new LinkedHashMap<>();
        source.forEach(
                (key, value) -> result.put(key, value == null ? null : String.valueOf(value)));
        return result;
    }
}
