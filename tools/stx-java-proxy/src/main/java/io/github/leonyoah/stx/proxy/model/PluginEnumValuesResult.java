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

package io.github.leonyoah.stx.proxy.model;

import java.util.Collections;
import java.util.List;

public class PluginEnumValuesResult {

    private final boolean ok;
    private final String pluginType;
    private final String factoryIdentifier;
    private final String optionKey;
    private final List<String> enumValues;
    private final List<String> warnings;

    public PluginEnumValuesResult(
            boolean ok,
            String pluginType,
            String factoryIdentifier,
            String optionKey,
            List<String> enumValues,
            List<String> warnings) {
        this.ok = ok;
        this.pluginType = pluginType;
        this.factoryIdentifier = factoryIdentifier;
        this.optionKey = optionKey;
        this.enumValues = Collections.unmodifiableList(enumValues);
        this.warnings = Collections.unmodifiableList(warnings);
    }

    public boolean isOk() {
        return ok;
    }

    public String getPluginType() {
        return pluginType;
    }

    public String getFactoryIdentifier() {
        return factoryIdentifier;
    }

    public String getOptionKey() {
        return optionKey;
    }

    public List<String> getEnumValues() {
        return enumValues;
    }

    public List<String> getWarnings() {
        return warnings;
    }
}
