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

import java.util.ArrayList;
import java.util.List;

public class PluginOptionDescriptor {

    private String key;
    private String type;
    private String elementType;
    private Object defaultValue;
    private String description;
    private List<String> fallbackKeys = new ArrayList<>();
    private List<String> enumValues = new ArrayList<>();
    private List<String> enumDisplayValues = new ArrayList<>();
    private RequiredMode requiredMode;
    private String conditionExpression;
    private String constraintGroup;
    private List<OptionOrigin> origins = new ArrayList<>();
    private List<String> declaredClasses = new ArrayList<>();
    private boolean advanced;

    public String getKey() {
        return key;
    }

    public void setKey(String key) {
        this.key = key;
    }

    public String getType() {
        return type;
    }

    public void setType(String type) {
        this.type = type;
    }

    public String getElementType() {
        return elementType;
    }

    public void setElementType(String elementType) {
        this.elementType = elementType;
    }

    public Object getDefaultValue() {
        return defaultValue;
    }

    public void setDefaultValue(Object defaultValue) {
        this.defaultValue = defaultValue;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public List<String> getFallbackKeys() {
        return fallbackKeys;
    }

    public void setFallbackKeys(List<String> fallbackKeys) {
        this.fallbackKeys = fallbackKeys;
    }

    public List<String> getEnumValues() {
        return enumValues;
    }

    public void setEnumValues(List<String> enumValues) {
        this.enumValues = enumValues;
    }

    public List<String> getEnumDisplayValues() {
        return enumDisplayValues;
    }

    public void setEnumDisplayValues(List<String> enumDisplayValues) {
        this.enumDisplayValues = enumDisplayValues;
    }

    public RequiredMode getRequiredMode() {
        return requiredMode;
    }

    public void setRequiredMode(RequiredMode requiredMode) {
        this.requiredMode = requiredMode;
    }

    public String getConditionExpression() {
        return conditionExpression;
    }

    public void setConditionExpression(String conditionExpression) {
        this.conditionExpression = conditionExpression;
    }

    public String getConstraintGroup() {
        return constraintGroup;
    }

    public void setConstraintGroup(String constraintGroup) {
        this.constraintGroup = constraintGroup;
    }

    public List<OptionOrigin> getOrigins() {
        return origins;
    }

    public void setOrigins(List<OptionOrigin> origins) {
        this.origins = origins;
    }

    public List<String> getDeclaredClasses() {
        return declaredClasses;
    }

    public void setDeclaredClasses(List<String> declaredClasses) {
        this.declaredClasses = declaredClasses;
    }

    public boolean isAdvanced() {
        return advanced;
    }

    public void setAdvanced(boolean advanced) {
        this.advanced = advanced;
    }
}
