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

/**
 * config tool 返回的 CatalogTable 结构解析（从原 WebUiDagPreview 迁移）。
 * Parsing of the CatalogTable shape returned by the config tool (moved from the former WebUiDagPreview).
 */

import type {SyncJSON, SyncSinkSaveModePreviewTable} from '@/lib/services/sync';
import type {DagVertex} from './dag-model';

export interface SchemaColumnDetail {
  name: string;
  dataType: string;
  nullable: boolean | null;
  defaultValue: string;
  comment: string;
  primaryKey: boolean;
  uniqueKey: boolean;
}

export interface TableSchemaSummary {
  comment: string;
  partitionKeys: string[];
  primaryKey: string[];
  constraintTypes: string[];
  columns: SchemaColumnDetail[];
}

function toObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function toStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map((item) => String(item)) : [];
}

/**
 * 解析单张表的结构；没有完整 schema 时返回 null，由调用方退回只显示列名。
 * Parse one table schema; returns null without a full schema so callers fall back to plain column names.
 */
export function summarizeTableSchema(schema?: SyncJSON): TableSchemaSummary | null {
  const root = toObject(schema);
  const schemaObject = toObject(root.schema);
  const rawColumns = Array.isArray(schemaObject.columns) ? schemaObject.columns : [];
  if (!schema || rawColumns.length === 0) {
    return null;
  }
  const primaryKey = toStringArray(toObject(schemaObject.primaryKey).columnNames);
  const primarySet = new Set(primaryKey);
  const uniqueSet = new Set<string>();
  const constraints = Array.isArray(schemaObject.constraintKeys) ? schemaObject.constraintKeys : [];
  const constraintTypes: string[] = [];
  for (const constraint of constraints) {
    const constraintObject = toObject(constraint);
    const type = String(constraintObject.constraintType || 'CONSTRAINT');
    constraintTypes.push(type);
    if (type !== 'UNIQUE_KEY' || !Array.isArray(constraintObject.columns)) {
      continue;
    }
    for (const column of constraintObject.columns) {
      const columnName = String(toObject(column).columnName || '').trim();
      if (columnName) {
        uniqueSet.add(columnName);
      }
    }
  }
  return {
    comment: String(root.comment || ''),
    partitionKeys: toStringArray(root.partitionKeys),
    primaryKey,
    constraintTypes,
    columns: rawColumns.map((column) => {
      const columnObject = toObject(column);
      const name = String(columnObject.name || '-');
      return {
        name,
        dataType: String(columnObject.dataType || '-'),
        nullable: typeof columnObject.nullable === 'boolean' ? columnObject.nullable : null,
        defaultValue:
          columnObject.defaultValue === null || typeof columnObject.defaultValue === 'undefined'
            ? '-'
            : String(columnObject.defaultValue),
        comment: String(columnObject.comment || ''),
        primaryKey: primarySet.has(name),
        uniqueKey: uniqueSet.has(name),
      };
    }),
  };
}

/**
 * 取某张表的建表计划；多表 Sink 没有逐表计划时退回 __default__。
 * Resolve the save-mode plan of one table; multi-table sinks without per-table plans fall back to __default__.
 */
export function resolveSaveModePlan(vertex: DagVertex, tablePath: string): SyncSinkSaveModePreviewTable | undefined {
  const previews = vertex.saveModePreviews || {};
  return previews[tablePath] || previews.__default__;
}
