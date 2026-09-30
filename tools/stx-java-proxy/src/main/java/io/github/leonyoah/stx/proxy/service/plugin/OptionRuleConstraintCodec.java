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

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.seatunnel.api.configuration.Option;
import org.apache.seatunnel.api.configuration.util.OptionRule;

import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * 将 3.0 OptionRule 的 valueConstraints / conditionRules 序列化为可 JSON 透传的结构； 2.x 无对应 getter
 * 时返回空列表（反射探测，保证双代际同一份源码可编译）。
 *
 * <p>Serializes 3.0 OptionRule valueConstraints / conditionRules into JSON-friendly maps; returns
 * empty lists on 2.x where getters are absent (reflection keeps one source compiling for both
 * epochs).
 */
final class OptionRuleConstraintCodec {

    private static final int MAX_CONDITION_CHAIN = 64;
    private static final int MAX_CONDITION_RULE_DEPTH = 8;

    private OptionRuleConstraintCodec() {}

    static List<Map<String, Object>> encodeValueConstraints(OptionRule optionRule) {
        List<?> constraints = invokeList(optionRule, "getValueConstraints");
        List<Map<String, Object>> encoded = new ArrayList<>();
        for (Object constraint : constraints) {
            Map<String, Object> node = encodeCondition(constraint, 0);
            if (node != null && !node.isEmpty()) {
                encoded.add(node);
            }
        }
        return encoded;
    }

    static List<Map<String, Object>> encodeConditionRules(OptionRule optionRule) {
        return encodeConditionRules(optionRule, 0);
    }

    private static List<Map<String, Object>> encodeConditionRules(
            OptionRule optionRule, int depth) {
        if (optionRule == null || depth > MAX_CONDITION_RULE_DEPTH) {
            return Collections.emptyList();
        }
        List<?> rules = invokeList(optionRule, "getConditionRules");
        List<Map<String, Object>> encoded = new ArrayList<>();
        for (Object rule : rules) {
            Map<String, Object> item = new LinkedHashMap<>();
            Object expression = invoke(rule, "getExpression");
            if (expression != null) {
                item.put("expression", String.valueOf(expression));
                Map<String, Object> tree = encodeExpression(expression, 0);
                if (tree != null && !tree.isEmpty()) {
                    item.put("expressionTree", tree);
                }
            }
            Object nestedRule = invoke(rule, "getOptionRule");
            if (nestedRule instanceof OptionRule) {
                OptionRule nested = (OptionRule) nestedRule;
                Map<String, Object> nestedPayload = new LinkedHashMap<>();
                nestedPayload.put("valueConstraints", encodeValueConstraints(nested));
                nestedPayload.put("conditionRules", encodeConditionRules(nested, depth + 1));
                // 子规则也展平 optional/required 的 key，便于补全过滤。
                // Also flatten nested option keys for completion filtering.
                nestedPayload.put("optionKeys", collectOptionKeys(nested));
                item.put("optionRule", nestedPayload);
            }
            if (!item.isEmpty()) {
                encoded.add(item);
            }
        }
        return encoded;
    }

    private static List<String> collectOptionKeys(OptionRule optionRule) {
        List<String> keys = new ArrayList<>();
        if (optionRule.getOptionalOptions() != null) {
            for (Option<?> option : optionRule.getOptionalOptions()) {
                if (option != null && StringUtils.isNotBlank(option.key())) {
                    keys.add(option.key());
                }
            }
        }
        if (optionRule.getRequiredOptions() != null) {
            for (Object required : optionRule.getRequiredOptions()) {
                Object options = invoke(required, "getOptions");
                if (!(options instanceof List)) {
                    continue;
                }
                for (Object optionObj : (List<?>) options) {
                    if (optionObj instanceof Option) {
                        String key = ((Option<?>) optionObj).key();
                        if (StringUtils.isNotBlank(key)) {
                            keys.add(key);
                        }
                    }
                }
            }
        }
        return keys;
    }

    private static Map<String, Object> encodeExpression(Object expression, int depth) {
        if (expression == null || depth > MAX_CONDITION_CHAIN) {
            return null;
        }
        Map<String, Object> node = new LinkedHashMap<>();
        Object condition = invoke(expression, "getCondition");
        Map<String, Object> conditionNode = encodeCondition(condition, 0);
        if (conditionNode != null) {
            node.put("condition", conditionNode);
        }
        Object andFlag = invoke(expression, "and");
        if (andFlag instanceof Boolean) {
            node.put("and", andFlag);
        }
        Object next = invoke(expression, "getNext");
        if (next != null) {
            Map<String, Object> nextNode = encodeExpression(next, depth + 1);
            if (nextNode != null && !nextNode.isEmpty()) {
                node.put("next", nextNode);
            }
        }
        return node;
    }

    private static Map<String, Object> encodeCondition(Object condition, int depth) {
        if (condition == null || depth > MAX_CONDITION_CHAIN) {
            return null;
        }
        Map<String, Object> node = new LinkedHashMap<>();
        Object option = invoke(condition, "getOption");
        if (option instanceof Option) {
            node.put("optionKey", ((Option<?>) option).key());
        }
        Object operator = invoke(condition, "getOperator");
        if (operator != null) {
            node.put("operator", String.valueOf(operator));
        }
        Object expectValue = invoke(condition, "getExpectValue");
        if (expectValue != null) {
            node.put("expectValue", stringifyValue(expectValue));
        }
        Object compareOption = invoke(condition, "getCompareOption");
        if (compareOption instanceof Option) {
            node.put("compareOptionKey", ((Option<?>) compareOption).key());
        }
        Object extension = invoke(condition, "getExtension");
        if (extension != null) {
            Object description = invoke(extension, "description");
            if (description != null) {
                node.put("extensionDescription", String.valueOf(description));
            }
        }
        Object andFlag = invoke(condition, "getAnd");
        if (andFlag instanceof Boolean) {
            node.put("and", andFlag);
        }
        Object next = invoke(condition, "getNext");
        if (next != null) {
            Map<String, Object> nextNode = encodeCondition(next, depth + 1);
            if (nextNode != null && !nextNode.isEmpty()) {
                node.put("next", nextNode);
            }
        }
        return node;
    }

    private static Object stringifyValue(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Enum) {
            return ((Enum<?>) value).name();
        }
        if (value instanceof Number || value instanceof Boolean || value instanceof String) {
            return value;
        }
        return String.valueOf(value);
    }

    private static List<?> invokeList(Object target, String methodName) {
        Object result = invoke(target, methodName);
        if (result instanceof List) {
            return (List<?>) result;
        }
        return Collections.emptyList();
    }

    private static Object invoke(Object target, String methodName) {
        if (target == null || StringUtils.isBlank(methodName)) {
            return null;
        }
        try {
            Method method = target.getClass().getMethod(methodName);
            return method.invoke(target);
        } catch (NoSuchMethodException e) {
            return null;
        } catch (Exception e) {
            return null;
        }
    }
}
