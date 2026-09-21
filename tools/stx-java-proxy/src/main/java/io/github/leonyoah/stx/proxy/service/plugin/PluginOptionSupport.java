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

package io.github.leonyoah.stx.proxy.service.plugin;

import org.apache.seatunnel.shade.com.fasterxml.jackson.core.type.TypeReference;
import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.seatunnel.api.configuration.Option;
import org.apache.seatunnel.api.configuration.SingleChoiceOption;

import io.github.leonyoah.stx.proxy.model.OptionOrigin;
import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.RequiredMode;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.ParameterizedType;
import java.lang.reflect.Type;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collection;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;

final class PluginOptionSupport {

    private PluginOptionSupport() {}

    static PluginOptionDescriptor buildDescriptor(
            Option<?> option,
            RequiredMode requiredMode,
            String conditionExpression,
            String constraintGroup,
            OptionOrigin origin,
            String declaredClass,
            boolean advanced) {
        PluginOptionDescriptor descriptor = new PluginOptionDescriptor();
        descriptor.setKey(option.key());
        descriptor.setType(resolveType(option.typeReference()));
        descriptor.setElementType(resolveElementType(option.typeReference()));
        descriptor.setDefaultValue(resolveDefaultValue(option.defaultValue()));
        descriptor.setDescription(option.getDescription());
        descriptor.setFallbackKeys(new ArrayList<>(option.getFallbackKeys()));
        descriptor.setEnumValues(resolveEnumValues(option));
        descriptor.setEnumDisplayValues(resolveEnumDisplayValues(option));
        descriptor.setRequiredMode(requiredMode);
        descriptor.setConditionExpression(conditionExpression);
        descriptor.setConstraintGroup(constraintGroup);
        descriptor.setOrigins(new ArrayList<>(Arrays.asList(origin)));
        descriptor.setDeclaredClasses(new ArrayList<>(Arrays.asList(declaredClass)));
        descriptor.setAdvanced(advanced);
        return descriptor;
    }

    static void merge(PluginOptionDescriptor target, PluginOptionDescriptor supplement) {
        if (target == null || supplement == null) {
            return;
        }
        if (StringUtils.isBlank(target.getDescription())
                && StringUtils.isNotBlank(supplement.getDescription())) {
            target.setDescription(supplement.getDescription());
        }
        if (target.getDefaultValue() == null && supplement.getDefaultValue() != null) {
            target.setDefaultValue(supplement.getDefaultValue());
        }
        if (StringUtils.isBlank(target.getType()) && StringUtils.isNotBlank(supplement.getType())) {
            target.setType(supplement.getType());
        }
        if (StringUtils.isBlank(target.getElementType())
                && StringUtils.isNotBlank(supplement.getElementType())) {
            target.setElementType(supplement.getElementType());
        }
        if (StringUtils.isBlank(target.getConditionExpression())
                && StringUtils.isNotBlank(supplement.getConditionExpression())) {
            target.setConditionExpression(supplement.getConditionExpression());
        }
        if (StringUtils.isBlank(target.getConstraintGroup())
                && StringUtils.isNotBlank(supplement.getConstraintGroup())) {
            target.setConstraintGroup(supplement.getConstraintGroup());
        }
        target.setFallbackKeys(
                mergeStrings(target.getFallbackKeys(), supplement.getFallbackKeys()));
        target.setEnumValues(mergeStrings(target.getEnumValues(), supplement.getEnumValues()));
        target.setEnumDisplayValues(
                mergeStrings(target.getEnumDisplayValues(), supplement.getEnumDisplayValues()));
        target.setDeclaredClasses(
                mergeStrings(target.getDeclaredClasses(), supplement.getDeclaredClasses()));
        target.setOrigins(mergeOrigins(target.getOrigins(), supplement.getOrigins()));
    }

    static List<String> mergeStrings(List<String> left, List<String> right) {
        LinkedHashSet<String> merged = new LinkedHashSet<>();
        if (left != null) {
            for (String value : left) {
                if (StringUtils.isNotBlank(value)) {
                    merged.add(value);
                }
            }
        }
        if (right != null) {
            for (String value : right) {
                if (StringUtils.isNotBlank(value)) {
                    merged.add(value);
                }
            }
        }
        return new ArrayList<>(merged);
    }

    static List<OptionOrigin> mergeOrigins(List<OptionOrigin> left, List<OptionOrigin> right) {
        LinkedHashSet<OptionOrigin> merged = new LinkedHashSet<>();
        if (left != null) {
            merged.addAll(left);
        }
        if (right != null) {
            merged.addAll(right);
        }
        return new ArrayList<>(merged);
    }

    static String resolveType(TypeReference<?> typeReference) {
        Type type = typeReference == null ? null : typeReference.getType();
        if (type instanceof Class) {
            Class<?> clazz = (Class<?>) type;
            if (String.class.equals(clazz)) {
                return "string";
            }
            if (Boolean.class.equals(clazz) || boolean.class.equals(clazz)) {
                return "boolean";
            }
            if (Integer.class.equals(clazz) || int.class.equals(clazz)) {
                return "int";
            }
            if (Long.class.equals(clazz) || long.class.equals(clazz)) {
                return "long";
            }
            if (Float.class.equals(clazz) || float.class.equals(clazz)) {
                return "float";
            }
            if (Double.class.equals(clazz) || double.class.equals(clazz)) {
                return "double";
            }
            if (clazz.isEnum()) {
                return "enum";
            }
            if (Map.class.isAssignableFrom(clazz)) {
                return "map";
            }
            if (Collection.class.isAssignableFrom(clazz) || clazz.isArray()) {
                return "list";
            }
            return "object";
        }
        if (type instanceof ParameterizedType) {
            ParameterizedType parameterizedType = (ParameterizedType) type;
            Type rawType = parameterizedType.getRawType();
            if (rawType instanceof Class) {
                Class<?> clazz = (Class<?>) rawType;
                if (Map.class.isAssignableFrom(clazz)) {
                    return "map";
                }
                if (Collection.class.isAssignableFrom(clazz)) {
                    return "list";
                }
            }
            return "object";
        }
        return "unknown";
    }

    static String resolveElementType(TypeReference<?> typeReference) {
        Type type = typeReference == null ? null : typeReference.getType();
        if (type instanceof ParameterizedType) {
            ParameterizedType parameterizedType = (ParameterizedType) type;
            Type[] arguments = parameterizedType.getActualTypeArguments();
            if (arguments.length == 1) {
                return normalizeTypeName(arguments[0]);
            }
            if (arguments.length > 1) {
                List<String> names = new ArrayList<>();
                for (Type argument : arguments) {
                    names.add(normalizeTypeName(argument));
                }
                return String.join(",", names);
            }
        }
        return null;
    }

    private static boolean isBooleanType(Type type, Option<?> option) {
        if (type == Boolean.class || type == boolean.class) {
            return true;
        }
        if (option != null && "boolean".equalsIgnoreCase(resolveType(option.typeReference()))) {
            return true;
        }
        return false;
    }

    static List<String> resolveEnumValues(Option<?> option) {
        if (option == null) {
            return new ArrayList<>();
        }
        LinkedHashSet<String> values = new LinkedHashSet<>();
        if (option instanceof SingleChoiceOption) {
            for (Object value : ((SingleChoiceOption<?>) option).getOptionValues()) {
                values.add(String.valueOf(resolveConfigLiteral(value)));
            }
            return new ArrayList<>(values);
        }
        Type type = option.typeReference() == null ? null : option.typeReference().getType();
        if (isBooleanType(type, option)) {
            values.add("true");
            values.add("false");
            return new ArrayList<>(values);
        }
        if (type instanceof Class && ((Class<?>) type).isEnum()) {
            Object[] constants = ((Class<?>) type).getEnumConstants();
            if (constants != null) {
                for (Object constant : constants) {
                    values.add(String.valueOf(resolveConfigLiteral(constant)));
                }
            }
        }
        return new ArrayList<>(values);
    }

    static List<String> resolveEnumDisplayValues(Option<?> option) {
        if (option == null) {
            return new ArrayList<>();
        }
        LinkedHashSet<String> values = new LinkedHashSet<>();
        if (option instanceof SingleChoiceOption) {
            for (Object value : ((SingleChoiceOption<?>) option).getOptionValues()) {
                values.add(String.valueOf(resolveDisplayValue(value)));
            }
            return new ArrayList<>(values);
        }
        Type type = option.typeReference() == null ? null : option.typeReference().getType();
        if (isBooleanType(type, option)) {
            values.add("true");
            values.add("false");
            return new ArrayList<>(values);
        }
        if (type instanceof Class && ((Class<?>) type).isEnum()) {
            Object[] constants = ((Class<?>) type).getEnumConstants();
            if (constants != null) {
                for (Object constant : constants) {
                    values.add(String.valueOf(resolveDisplayValue(constant)));
                }
            }
        }
        return new ArrayList<>(values);
    }

    static String renderValue(PluginOptionDescriptor descriptor) {
        Object defaultValue = descriptor.getDefaultValue();
        if (defaultValue != null) {
            return renderLiteral(defaultValue);
        }
        String type = StringUtils.defaultString(descriptor.getType()).toLowerCase(Locale.ROOT);
        switch (type) {
            case "string":
                return "\"\"";
            case "list":
                return "[]";
            case "map":
            case "object":
                return "{}";
            default:
                return "null";
        }
    }

    private static String renderLiteral(Object value) {
        if (value == null) {
            return "null";
        }
        if (value instanceof String) {
            return quote((String) value);
        }
        if (value instanceof Number || value instanceof Boolean) {
            return String.valueOf(value);
        }
        if (value instanceof Enum) {
            return renderLiteral(resolveConfigLiteral(value));
        }
        if (value instanceof Collection) {
            List<String> rendered = new ArrayList<>();
            for (Object item : (Collection<?>) value) {
                rendered.add(renderLiteral(item));
            }
            return "[" + String.join(", ", rendered) + "]";
        }
        if (value instanceof Map) {
            List<String> rendered = new ArrayList<>();
            for (Map.Entry<?, ?> entry : ((Map<?, ?>) value).entrySet()) {
                rendered.add(
                        String.valueOf(entry.getKey()) + " = " + renderLiteral(entry.getValue()));
            }
            return "{" + String.join(", ", rendered) + "}";
        }
        if (value.getClass().isArray()) {
            return renderLiteral(Arrays.asList((Object[]) value));
        }
        return quote(String.valueOf(value));
    }

    private static Object resolveDefaultValue(Object value) {
        if (value instanceof Enum) {
            return resolveConfigLiteral(value);
        }
        return value;
    }

    static Object resolveConfigLiteral(Object value) {
        if (!(value instanceof Enum)) {
            return value;
        }
        Object extracted =
                firstNonNull(
                        invokeAccessor(value, "getValue"),
                        invokeAccessor(value, "value"),
                        invokeAccessor(value, "getPattern"),
                        invokeAccessor(value, "pattern"),
                        invokeAccessor(value, "getFormat"),
                        invokeAccessor(value, "format"),
                        readField(value, "value"),
                        readField(value, "pattern"),
                        readField(value, "format"));
        if (extracted != null && extracted != value) {
            return extracted;
        }
        String rendered = String.valueOf(value);
        if (value instanceof Enum) {
            String name = ((Enum<?>) value).name();
            if (!StringUtils.equals(rendered, name)) {
                return rendered;
            }
            return name;
        }
        return rendered;
    }

    private static Object resolveDisplayValue(Object value) {
        if (value instanceof Enum) {
            return ((Enum<?>) value).name();
        }
        return value;
    }

    private static Object invokeAccessor(Object target, String methodName) {
        try {
            Method method = target.getClass().getMethod(methodName);
            method.setAccessible(true);
            return method.invoke(target);
        } catch (Exception ignored) {
            try {
                Method method = target.getClass().getDeclaredMethod(methodName);
                method.setAccessible(true);
                return method.invoke(target);
            } catch (Exception ignoredAgain) {
                return null;
            }
        }
    }

    private static Object readField(Object target, String fieldName) {
        try {
            Field field = target.getClass().getField(fieldName);
            field.setAccessible(true);
            return field.get(target);
        } catch (Exception ignored) {
            try {
                Field field = target.getClass().getDeclaredField(fieldName);
                field.setAccessible(true);
                return field.get(target);
            } catch (Exception ignoredAgain) {
                return null;
            }
        }
    }

    private static Object firstNonNull(Object... values) {
        for (Object value : values) {
            if (value != null) {
                return value;
            }
        }
        return null;
    }

    private static String normalizeTypeName(Type type) {
        if (type instanceof Class) {
            Class<?> clazz = (Class<?>) type;
            if (clazz.isEnum()) {
                return "enum";
            }
            return clazz.getSimpleName().toLowerCase(Locale.ROOT);
        }
        return type == null ? null : type.getTypeName();
    }

    private static String quote(String value) {
        StringBuilder sanitized = new StringBuilder();
        String source = StringUtils.defaultString(value);
        for (int i = 0; i < source.length(); i++) {
            char ch = source.charAt(i);
            switch (ch) {
                case '\\':
                    sanitized.append("\\\\");
                    break;
                case '"':
                    sanitized.append("\\\"");
                    break;
                case '\n':
                    sanitized.append("\\n");
                    break;
                case '\r':
                    sanitized.append("\\r");
                    break;
                case '\t':
                    sanitized.append("\\t");
                    break;
                case '\b':
                    sanitized.append("\\b");
                    break;
                case '\f':
                    sanitized.append("\\f");
                    break;
                default:
                    if (Character.isISOControl(ch)) {
                        sanitized.append(String.format("\\u%04x", (int) ch));
                    } else {
                        sanitized.append(ch);
                    }
                    break;
            }
        }
        return "\"" + sanitized + "\"";
    }
}
