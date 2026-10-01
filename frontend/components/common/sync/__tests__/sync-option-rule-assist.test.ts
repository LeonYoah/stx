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

import {describe, expect, it} from 'vitest';
import {
  buildOptionRuleDiagnostics,
  evaluateConditionExpressionString,
  extractPluginBlockSnapshots,
  rankOptionKeyCompletions,
  resolveOptionKeyCompletionContext,
} from '../sync-option-rule-assist';

const SAMPLE = `
env {
  job.mode = "BATCH"
}
source {
  FakeSource {
    plugin_output = "fake"
    mode = TIMESTAMP
    port = 70000
    timeout = {{timeout}}
  }
}
sink {
  Console {
    plugin_input = ["fake"]
  }
}
`;

describe('sync-option-rule-assist', () => {
  it('extracts plugin block assignments and skips templates', () => {
    const snapshots = extractPluginBlockSnapshots(SAMPLE);
    expect(snapshots).toHaveLength(2);
    const source = snapshots[0];
    expect(source.factoryIdentifier).toBe('FakeSource');
    expect(source.values.mode).toBe('TIMESTAMP');
    expect(source.values.port).toBe('70000');
    expect(source.values.timeout).toBeNull();
  });

  it('builds value constraint diagnostics for failing nodes', () => {
    const snapshot = extractPluginBlockSnapshots(SAMPLE)[0];
    const diagnostics = buildOptionRuleDiagnostics(
      {
        options: [{key: 'port'}, {key: 'mode'}],
        value_constraints: [
          {
            optionKey: 'port',
            operator: 'GREATER_OR_EQUAL',
            expectValue: 1,
            and: true,
            next: {
              optionKey: 'port',
              operator: 'LESS_OR_EQUAL',
              expectValue: 65535,
            },
          },
        ],
      },
      snapshot,
    );
    expect(diagnostics.some((item) => item.message.includes('≤'))).toBe(true);
    expect(diagnostics.some((item) => item.message.includes('≥'))).toBe(false);
  });

  it('ranks conditionRules keys when mode matches', () => {
    const ranks = rankOptionKeyCompletions(
      {
        options: [
          {key: 'mode'},
          {key: 'plugin_output'},
          {key: 'timestamp'},
          {key: 'startup'},
        ],
        condition_rules: [
          {
            expression: "'mode' == TIMESTAMP",
            expressionTree: {
              condition: {
                optionKey: 'mode',
                operator: 'EQUAL',
                expectValue: 'TIMESTAMP',
              },
            },
            optionRule: {optionKeys: ['timestamp']},
          },
          {
            expression: "'mode' == EARLIEST",
            expressionTree: {
              condition: {
                optionKey: 'mode',
                operator: 'EQUAL',
                expectValue: 'EARLIEST',
              },
            },
            optionRule: {optionKeys: ['startup']},
          },
        ],
      },
      {mode: 'TIMESTAMP'},
    );
    const byKey = Object.fromEntries(ranks.map((item) => [item.key, item]));
    expect(byKey.timestamp.sortBucket).toBe(0);
    expect(byKey.startup.sortBucket).toBe(5);
    expect(byKey.plugin_output.sortBucket).toBe(2);
  });

  it('evaluates string condition expressions', () => {
    expect(
      evaluateConditionExpressionString("'mode' == TIMESTAMP", {
        mode: 'TIMESTAMP',
      }),
    ).toBe(true);
    expect(
      evaluateConditionExpressionString("'mode' == TIMESTAMP", {
        mode: 'EARLIEST',
      }),
    ).toBe(false);
  });

  it('detects option key completion region inside factory block', () => {
    const lines = SAMPLE.split('\n');
    const modeLine = lines.findIndex((line) => /^\s*mode\s*=/.test(line));
    // Monaco column 为 1-based；停在 "mo" 之后 → indexOf('mode') + 3。
    // Monaco columns are 1-based; cursor after "mo" → indexOf('mode') + 3.
    const column = lines[modeLine].indexOf('mode') + 3;
    const ctx = resolveOptionKeyCompletionContext(
      SAMPLE,
      modeLine + 1,
      column,
    );
    expect(ctx.inKeyRegion).toBe(true);
    expect(ctx.snapshot?.factoryIdentifier).toBe('FakeSource');
    expect(ctx.prefix).toBe('mo');
  });
});
