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

package io.github.leonyoah.stx.proxy.adapter;

import org.apache.seatunnel.engine.serializer.api.Serializer;
import org.apache.seatunnel.engine.serializer.protobuf.ProtoStuffSerializer;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.Callable;

/**
 * Base abstract engine adapter providing thread class loader contextual isolation, resilient
 * reflection utilities, and common WAL/storage decoding routines.
 */
public abstract class AbstractSeaTunnelEngineAdapter implements SeaTunnelEngineAdapter {

    protected final Logger log = LoggerFactory.getLogger(getClass());
    protected final Serializer protoSerializer = new ProtoStuffSerializer();

    private static final String IMAP_FILE_DATA_CLASS =
            "org.apache.seatunnel.engine.imap.storage.file.bean.IMapFileData";
    private static final int WAL_DATA_METADATA_LENGTH = 12;

    @Override
    public Map<String, Object> inspectIMapWal(
            Map<String, Object> request, byte[] rawBytes, ClassLoader classLoader)
            throws Exception {
        return runWithClassLoader(
                classLoader,
                () -> {
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
                        Object imapFileData = deserializeIMapFileData(recordBytes, classLoader);
                        entries.add(buildWalEntry(imapFileData, recordLength));
                    }

                    Map<String, Object> result = new LinkedHashMap<>();
                    result.put("adapterVersion", getAdapterVersion());
                    result.put("entryCount", entries.size());
                    result.put("entries", entries);
                    return result;
                });
    }

    protected <T> T runWithClassLoader(ClassLoader classLoader, Callable<T> task) throws Exception {
        ClassLoader original = Thread.currentThread().getContextClassLoader();
        ClassLoader target = classLoader != null ? classLoader : original;
        try {
            Thread.currentThread().setContextClassLoader(target);
            return task.call();
        } finally {
            Thread.currentThread().setContextClassLoader(original);
        }
    }

    protected Object deserializeIMapFileData(byte[] recordBytes, ClassLoader classLoader)
            throws Exception {
        Class<?> imapFileDataClass = Class.forName(IMAP_FILE_DATA_CLASS, true, classLoader);
        return protoSerializer.deserialize(recordBytes, imapFileDataClass);
    }

    protected Map<String, Object> buildWalEntry(Object imapFileData, int recordLength) {
        Map<String, Object> entry = new LinkedHashMap<>();
        entry.put("recordSizeBytes", recordLength);
        if (imapFileData == null) {
            return entry;
        }
        entry.put("key", readPropertyQuietly(imapFileData, "key"));
        entry.put("value", readPropertyQuietly(imapFileData, "value"));
        return entry;
    }

    protected Object readPropertyQuietly(Object target, String propertyName) {
        if (target == null || propertyName == null) {
            return null;
        }
        try {
            String getterName =
                    "get"
                            + Character.toUpperCase(propertyName.charAt(0))
                            + propertyName.substring(1);
            Method method = target.getClass().getMethod(getterName);
            return method.invoke(target);
        } catch (Exception ignored) {
        }
        try {
            Field field = target.getClass().getDeclaredField(propertyName);
            field.setAccessible(true);
            return field.get(target);
        } catch (Exception ignored) {
        }
        return null;
    }

    private static int byteArrayToInt(byte[] b, int offset) {
        return b[offset + 3] & 0xFF
                | (b[offset + 2] & 0xFF) << 8
                | (b[offset + 1] & 0xFF) << 16
                | (b[offset] & 0xFF) << 24;
    }
}
