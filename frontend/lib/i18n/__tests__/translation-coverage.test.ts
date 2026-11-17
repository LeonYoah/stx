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

import {readdirSync, readFileSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import ts from 'typescript';
import {describe, expect, it} from 'vitest';
import {createTranslator} from 'next-intl';
import zhMessages from '../locales/zh.json';
import enMessages from '../locales/en.json';

const frontendRoot = process.cwd();
const ignoredDirectories = new Set([
  '.next',
  'dist-standalone',
  'e2e',
  'node_modules',
  '__tests__',
]);

type MessageTree = Record<string, unknown>;

interface TranslationReference {
  file: string;
  line: number;
  key: string;
}

/**
 * 递归收集语言包的叶子键，确保深层文案也参与检查。
 * Collect leaf message keys recursively so nested translations are checked too.
 */
function collectLeafKeys(value: unknown, prefix = ''): string[] {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return Object.entries(value as MessageTree).flatMap(([key, child]) =>
      collectLeafKeys(child, prefix ? `${prefix}.${key}` : key),
    );
  }
  return [prefix];
}

/**
 * 读取指定文案键，返回 undefined 表示语言包缺少该路径。
 * Read a message path and return undefined when the locale does not contain it.
 */
function getMessage(messages: MessageTree, key: string): unknown {
  let current: unknown = messages;
  for (const segment of key.split('.')) {
    if (
      !current ||
      typeof current !== 'object' ||
      !Object.prototype.hasOwnProperty.call(current, segment)
    ) {
      return undefined;
    }
    current = (current as MessageTree)[segment];
  }
  return current;
}

/**
 * 收集前端生产代码，排除测试与构建产物。
 * Collect frontend production sources while excluding tests and build output.
 */
function collectSourceFiles(directory: string, files: string[] = []): string[] {
  for (const entry of readdirSync(directory, {withFileTypes: true})) {
    if (entry.isDirectory() && ignoredDirectories.has(entry.name)) {
      continue;
    }
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      collectSourceFiles(entryPath, files);
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      files.push(entryPath);
    }
  }
  return files;
}

/**
 * 动态文案键引用：模板里至少有一个无法在编译期确定取值的插值。
 * Dynamic message reference: the template has at least one hole whose
 * value cannot be resolved statically.
 */
interface DynamicTranslationReference {
  file: string;
  line: number;
  /** 形如 `volume.metrics.${key}` 的展示用模板。/ Display template like `volume.metrics.${key}`. */
  template: string;
  /** 匹配语言包叶子键的正则；插值位一律放宽为 `.+`。/ Regex matched against locale leaf keys; each hole becomes `.+`. */
  leafPattern: RegExp;
}

interface CollectedTranslationReferences {
  /** 字面量键与「所有插值都能展开成常量集合」的动态键。/ Literal keys plus dynamic keys whose holes all resolve to constant sets. */
  staticReferences: TranslationReference[];
  dynamicReferences: DynamicTranslationReference[];
}

/** t.rich / t.markup / t.raw / t.has 等挂在翻译函数上的方法名。/ Method names invoked on translator functions. */
const TRANSLATOR_METHODS = new Set(['rich', 'markup', 'raw', 'has']);

/**
 * 去掉 as const / satisfies / 括号等包装，拿到表达式本体。
 * Unwrap as-const / satisfies / parentheses to reach the inner expression.
 */
function unwrapExpression(expression: ts.Expression): ts.Expression {
  let current = expression;
  while (
    ts.isAsExpression(current) ||
    ts.isSatisfiesExpression(current) ||
    ts.isParenthesizedExpression(current)
  ) {
    current = current.expression;
  }
  return current;
}

/**
 * 若数组字面量的元素全是字符串字面量则返回取值集合，否则返回 null。
 * Return the literal set when the array literal contains only string literals.
 */
function arrayLiteralValues(expression: ts.Expression): string[] | null {
  const inner = unwrapExpression(expression);
  if (!ts.isArrayLiteralExpression(inner)) {
    return null;
  }
  const values: string[] = [];
  for (const element of inner.elements) {
    if (!ts.isStringLiteralLike(element)) {
      return null;
    }
    values.push(element.text);
  }
  return values;
}

/**
 * 在文件内查找 `const 名 = [...]` 形式的字符串常量数组。
 * Find a `const name = [...]` string literal array declared in the file.
 */
function findConstArrayValues(
  sourceFile: ts.SourceFile,
  name: string,
): string[] | null {
  let found: string[] | null = null;
  function visit(node: ts.Node): void {
    if (found !== null) {
      return;
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer
    ) {
      found = arrayLiteralValues(node.initializer);
      return;
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
  return found;
}

/**
 * 若标识符是 `CONST.map((x) => ...)` 这类回调的参数，返回被遍历常量数组的取值。
 * When the identifier is the callback parameter of `CONST.map((x) => ...)`,
 * resolve it to the literals of the iterated constant array.
 */
function resolveCallbackParamValues(
  callSite: ts.Node,
  name: string,
  sourceFile: ts.SourceFile,
): string[] | null {
  let current: ts.Node | undefined = callSite.parent;
  while (current) {
    if (
      ts.isCallExpression(current) &&
      ts.isPropertyAccessExpression(current.expression) &&
      ts.isIdentifier(current.expression.expression)
    ) {
      const method = current.expression.name.text;
      if (['map', 'forEach', 'flatMap', 'some', 'every', 'filter', 'find'].includes(method)) {
        const callback = current.arguments.find(
          (arg): arg is ts.ArrowFunction | ts.FunctionExpression =>
            ts.isArrowFunction(arg) || ts.isFunctionExpression(arg),
        );
        const firstParam = callback?.parameters[0];
        if (
          firstParam &&
          ts.isIdentifier(firstParam.name) &&
          firstParam.name.text === name
        ) {
          return findConstArrayValues(
            sourceFile,
            current.expression.expression.text,
          );
        }
      }
    }
    current = current.parent;
  }
  return null;
}

/**
 * 尽力把插值表达式解析成字符串取值集合；解析不了返回 null。
 * Best-effort resolution of a template hole into a literal set; null when
 * the value is data-driven or otherwise not statically known.
 */
function resolveHoleValues(
  expression: ts.Expression,
  callSite: ts.Node,
  sourceFile: ts.SourceFile,
): string[] | null {
  if (ts.isStringLiteralLike(expression)) {
    return [expression.text];
  }
  if (ts.isIdentifier(expression)) {
    return (
      findConstArrayValues(sourceFile, expression.text) ??
      resolveCallbackParamValues(callSite, expression.text, sourceFile)
    );
  }
  if (ts.isConditionalExpression(expression)) {
    const whenTrue = resolveHoleValues(expression.whenTrue, callSite, sourceFile);
    const whenFalse = resolveHoleValues(expression.whenFalse, callSite, sourceFile);
    return whenTrue && whenFalse ? [...whenTrue, ...whenFalse] : null;
  }
  return null;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function cartesianProduct(sets: string[][]): string[][] {
  return sets.reduce<string[][]>(
    (acc, set) => acc.flatMap((prefix) => set.map((value) => [...prefix, value])),
    [[]],
  );
}

/**
 * 扫描 useTranslations 返回函数（含 t.rich / t.raw 等方法调用）引用的文案键。
 * 静态字面量直接收集；`t(`ns.${x}`)` 这类模板键能解析出常量集合的展开后并入
 * 静态结果，解析不了的记入动态引用交给正则族检查。
 * Scan message keys referenced through functions returned by useTranslations,
 * including method calls like t.rich / t.raw. Literal keys are collected
 * directly; template keys like t(`ns.${x}`) are expanded into static results
 * when every hole resolves to a constant set, otherwise recorded as dynamic
 * references for the regex family check below.
 */
function collectTranslationReferences(): CollectedTranslationReferences {
  const staticReferences: TranslationReference[] = [];
  const dynamicReferences: DynamicTranslationReference[] = [];

  for (const file of collectSourceFiles(frontendRoot)) {
    const sourceFile = ts.createSourceFile(
      file,
      readFileSync(file, 'utf8'),
      ts.ScriptTarget.Latest,
      true,
      file.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    );
    const translators = new Map<string, string>();

    function collectTranslators(node: ts.Node): void {
      if (
        ts.isVariableDeclaration(node) &&
        ts.isIdentifier(node.name) &&
        node.initializer
      ) {
        const initializer = unwrapExpression(node.initializer);
        const call = ts.isAwaitExpression(initializer)
          ? unwrapExpression(initializer.expression)
          : initializer;
        if (
          ts.isCallExpression(call) &&
          ts.isIdentifier(call.expression) &&
          (call.expression.text === 'useTranslations' ||
            call.expression.text === 'getTranslations')
        ) {
          const namespaceArgument = call.arguments[0];
          const namespace =
            namespaceArgument && ts.isStringLiteralLike(namespaceArgument)
              ? namespaceArgument.text
              : '';
          const previousNamespace = translators.get(node.name.text);
          if (
            previousNamespace !== undefined &&
            previousNamespace !== namespace
          ) {
            throw new Error(
              `${path.relative(frontendRoot, file)} 中的翻译函数 ${node.name.text} 使用了多个命名空间`,
            );
          }
          translators.set(node.name.text, namespace);
        }
      }
      ts.forEachChild(node, collectTranslators);
    }

    function collectCalls(node: ts.Node): void {
      if (!ts.isCallExpression(node)) {
        ts.forEachChild(node, collectCalls);
        return;
      }
      const callee = node.expression;
      let translatorName: string | undefined;
      if (ts.isIdentifier(callee) && translators.has(callee.text)) {
        translatorName = callee.text;
      } else if (
        ts.isPropertyAccessExpression(callee) &&
        ts.isIdentifier(callee.expression) &&
        translators.has(callee.expression.text) &&
        TRANSLATOR_METHODS.has(callee.name.text)
      ) {
        translatorName = callee.expression.text;
      }
      if (translatorName === undefined) {
        ts.forEachChild(node, collectCalls);
        return;
      }

      const namespace = translators.get(translatorName) || '';
      const prefix = namespace ? `${namespace}.` : '';
      const keyArgument = node.arguments[0];
      const line =
        sourceFile.getLineAndCharacterOfPosition(
          (keyArgument ?? node).getStart(),
        ).line + 1;
      const relativeFile = path.relative(frontendRoot, file);

      if (keyArgument && ts.isStringLiteralLike(keyArgument)) {
        staticReferences.push({
          file: relativeFile,
          line,
          key: `${prefix}${keyArgument.text}`,
        });
      } else if (keyArgument && ts.isNoSubstitutionTemplateLiteral(keyArgument)) {
        staticReferences.push({
          file: relativeFile,
          line,
          key: `${prefix}${keyArgument.text}`,
        });
      } else if (keyArgument && ts.isTemplateExpression(keyArgument)) {
        const holeValues: string[][] = [];
        let allResolved = true;
        let leafPattern = `^${escapeRegExp(prefix)}${escapeRegExp(keyArgument.head.text)}`;
        let template = `${prefix}${keyArgument.head.text}`;
        for (const span of keyArgument.templateSpans) {
          const resolved = resolveHoleValues(span.expression, node, sourceFile);
          if (resolved === null) {
            allResolved = false;
          }
          holeValues.push(resolved ?? []);
          leafPattern += `.+${escapeRegExp(span.literal.text)}`;
          template += `\${${span.expression.getText(sourceFile)}}${span.literal.text}`;
        }
        leafPattern += '$';

        if (allResolved) {
          // 所有插值都解析成常量集合：展开成具体键，走与字面量相同的严格检查。
          // Every hole resolved to a constant set: expand into concrete keys
          // and check them exactly like literal keys.
          for (const combination of cartesianProduct(holeValues)) {
            let key = `${prefix}${keyArgument.head.text}`;
            keyArgument.templateSpans.forEach((span, index) => {
              key += `${combination[index]}${span.literal.text}`;
            });
            staticReferences.push({file: relativeFile, line, key});
          }
        } else {
          dynamicReferences.push({
            file: relativeFile,
            line,
            template,
            leafPattern: new RegExp(leafPattern),
          });
        }
      }
      ts.forEachChild(node, collectCalls);
    }

    collectTranslators(sourceFile);
    collectCalls(sourceFile);
  }

  return {staticReferences, dynamicReferences};
}

/**
 * 提取 ICU 文案参数，防止中英文使用不同的占位符。
 * Extract ICU arguments so both locales use the same placeholders.
 */
function collectPlaceholders(message: string): string[] {
  return [...message.matchAll(/\{([A-Za-z0-9_]+)(?:,[^}]*)?\}/g)]
    .map((match) => match[1])
    .sort();
}

describe('translation coverage', () => {
  it('中英文语言包的所有深层键保持一致', () => {
    expect(collectLeafKeys(zhMessages).sort()).toEqual(
      collectLeafKeys(enMessages).sort(),
    );
  });

  it('生产代码引用的静态文案键在中英文语言包中都存在', () => {
    const {staticReferences} = collectTranslationReferences();
    const missing = staticReferences
      .filter(
        ({key}) =>
          getMessage(zhMessages, key) === undefined ||
          getMessage(enMessages, key) === undefined,
      )
      .map(
        ({file, line, key}) =>
          `${file}:${line} -> ${key} (zh=${getMessage(zhMessages, key) !== undefined}, en=${getMessage(enMessages, key) !== undefined})`,
      );

    expect(missing).toEqual([]);
  });

  it('解析不出常量集合的动态文案键，其键族在中英文语言包中都存在', () => {
    // 数据驱动的键（如 host.statuses.${status}、rule_${rule}）无法逐一枚举取值，
    // 但至少要求模板对应的叶子键族在语言包中真实存在，防止拼错命名空间或整组漏翻。
    // Data-driven keys (e.g. host.statuses.${status}, rule_${rule}) cannot be
    // enumerated exhaustively, so require the template's leaf family to exist in
    // each locale — this catches mistyped namespaces and missing key groups.
    const {dynamicReferences} = collectTranslationReferences();
    const zhLeaves = collectLeafKeys(zhMessages);
    const enLeaves = collectLeafKeys(enMessages);
    const missing = dynamicReferences
      .filter(
        ({leafPattern}) =>
          !zhLeaves.some((leaf) => leafPattern.test(leaf)) ||
          !enLeaves.some((leaf) => leafPattern.test(leaf)),
      )
      .map(
        ({file, line, template, leafPattern}) =>
          `${file}:${line} -> ${template} (zh=${zhLeaves.some((leaf) => leafPattern.test(leaf))}, en=${enLeaves.some((leaf) => leafPattern.test(leaf))})`,
      );

    expect(missing).toEqual([]);
  });

  it('中英文文案的 ICU 参数保持一致', () => {
    const mismatches = collectLeafKeys(zhMessages).flatMap((key) => {
      const zhMessage = getMessage(zhMessages, key);
      const enMessage = getMessage(enMessages, key);
      if (typeof zhMessage !== 'string' || typeof enMessage !== 'string') {
        return [];
      }
      const zhPlaceholders = collectPlaceholders(zhMessage);
      const enPlaceholders = collectPlaceholders(enMessage);
      return JSON.stringify(zhPlaceholders) === JSON.stringify(enPlaceholders)
        ? []
        : [
            `${key}: zh=${zhPlaceholders.join(',')} en=${enPlaceholders.join(',')}`,
          ];
    });

    expect(mismatches).toEqual([]);
  });

  it('所有文案能够被 next-intl 成功解析且无 MALFORMED_ARGUMENT 等语法错误', () => {
    // 验证所有消息都能被 next-intl 成功解析
    // Verify all messages can be successfully parsed by next-intl
    const invalidMessages: string[] = [];

    const tZh = createTranslator({
      locale: 'zh',
      messages: zhMessages,
      onError(err) {
        if (err.code === 'INVALID_MESSAGE') {
          invalidMessages.push(`zh: ${err.message}`);
        }
      },
    });
    const tEn = createTranslator({
      locale: 'en',
      messages: enMessages,
      onError(err) {
        if (err.code === 'INVALID_MESSAGE') {
          invalidMessages.push(`en: ${err.message}`);
        }
      },
    });

    for (const key of collectLeafKeys(zhMessages)) {
      tZh(key as never);
      tEn(key as never);
    }

    expect(invalidMessages).toEqual([]);
  });
});

