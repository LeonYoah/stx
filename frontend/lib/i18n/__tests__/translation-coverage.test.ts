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
 * 扫描 useTranslations 返回函数所引用的静态文案键。
 * Scan static message keys called through functions returned by useTranslations.
 */
function collectStaticTranslationReferences(): TranslationReference[] {
  const references: TranslationReference[] = [];

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
        node.initializer &&
        ts.isCallExpression(node.initializer) &&
        ts.isIdentifier(node.initializer.expression) &&
        node.initializer.expression.text === 'useTranslations'
      ) {
        const namespaceArgument = node.initializer.arguments[0];
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
      ts.forEachChild(node, collectTranslators);
    }

    function collectCalls(node: ts.Node): void {
      if (
        ts.isCallExpression(node) &&
        ts.isIdentifier(node.expression) &&
        translators.has(node.expression.text)
      ) {
        const keyArgument = node.arguments[0];
        if (keyArgument && ts.isStringLiteralLike(keyArgument)) {
          const namespace = translators.get(node.expression.text) || '';
          references.push({
            file: path.relative(frontendRoot, file),
            line:
              sourceFile.getLineAndCharacterOfPosition(keyArgument.getStart())
                .line + 1,
            key: namespace
              ? `${namespace}.${keyArgument.text}`
              : keyArgument.text,
          });
        }
      }
      ts.forEachChild(node, collectCalls);
    }

    collectTranslators(sourceFile);
    collectCalls(sourceFile);
  }

  return references;
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
    const missing = collectStaticTranslationReferences()
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
});
